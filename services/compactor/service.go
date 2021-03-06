package compactor

import (
	"context"
	"errors"
	"time"

	redigo "github.com/garyburd/redigo/redis"
	"github.com/koding/redis"
	"github.com/ropelive/count/pkg"
	"github.com/ropelive/count/pkg/mongodb"
)

// Service is a simple interface for compactor operations.
type Service interface {
	Process(ctx context.Context, p ProcessRequest) error
}

type compactorService struct {
	app *pkg.App
}

// NewService creates a Compator service
func NewService(app *pkg.App) Service {
	return &compactorService{
		app: app,
	}
}

// Process
func (c *compactorService) Process(ctx context.Context, p ProcessRequest) error {
	c.app.Logger.Log("starttime", p.StartAt.Format(time.RFC3339))

	tr := pkg.GetLastProcessibleSegment(p.StartAt)
	tl := tr.Add(-time.Hour) // / process till this time

	redisConn := c.app.MustGetRedis()
	redisConn.SetPrefix("ropecount")

	for tl.UnixNano() <= tr.UnixNano() {
		c.app.InfoLog("time", tr.Format(time.RFC3339))

		keyNames := pkg.GenerateKeyNames(tr)
		for {
			var srcErr, dstErr error
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				srcErr = c.process(redisConn, keyNames.Src, tr)
				dstErr = c.process(redisConn, keyNames.Dst, tr)
			}
			if srcErr == dstErr && srcErr == errNotFound {
				break
			}
			if srcErr != nil && srcErr != errNotFound {
				return srcErr
			}
			if dstErr != nil && dstErr != errNotFound {
				return dstErr
			}
		}

		tr = tr.Add(-pkg.SegmentDur)
	}

	return nil
}

var errNotFound = errors.New("no item to process")

func (c *compactorService) process(redisConn *redis.RedisSession, keyNames pkg.KeyNames, tr time.Time) error {
	c.app.InfoLog("current_counter_queue", keyNames.CurrentCounterSet)
	return c.withLock(redisConn, keyNames.CurrentCounterSet, func(srcMember string) error {
		source := keyNames.HashSetName(srcMember)
		return c.merge(redisConn, source)
	})
}

// withLock gets an item from the current segment's item set and  passes it to
// the given processor function. After getting a response from the processor a successfull
func (c *compactorService) withLock(redisConn *redis.RedisSession, queueName string, fn func(srcMember string) error) error {
	srcMember, err := redisConn.RandomSetMember(queueName)
	if err == redis.ErrNil {
		return errNotFound // we dont have any, so nothing to do.
	}

	if err != nil {
		return err
	}

	processingQueueName := queueName + "_processing"

	res, err := redisConn.MoveSetMember(queueName, processingQueueName, srcMember)
	if err != nil {
		return err
	}

	// 1 if the element is moved. 0 if the element is not a member of source
	// and no operation was performed.
	if res != 1 {
		c.app.InfoLog("msg", "we tried to move a current member to processing queue but failed, someone has already moved the item in the mean time...")
		return c.withLock(redisConn, queueName, fn)
	}

	fnErr := fn(srcMember)

	if fnErr != nil {
		_, err = redisConn.MoveSetMember(processingQueueName, queueName, srcMember)
		if err != nil {
			c.app.ErrorLog("msg", "error while trying to put to item back to process set after an unseccesful operation", "err", err.Error())
		}

		return fnErr
	}

	res, err = redisConn.RemoveSetMembers(processingQueueName, srcMember)
	if err == redis.ErrNil {
		c.app.ErrorLog("msg", "we should be able to delete from the processing set here, but failed.")
		return err
	}

	if err != nil {
		c.app.ErrorLog("msg", err.Error())
		return err
	}

	if res == 0 {
		c.app.ErrorLog("msg", "we should be able to delete from the processing set here, but failed.")
	}

	return nil
}

// merge merges the source hash map values to the target, then deletes the
// source hash map from the server.
func (c *compactorService) merge(redisConn *redis.RedisSession, source string) error {
	fns, err := redigo.Int64Map(redisConn.HashGetAll(source))
	if err == redis.ErrNil {
		c.app.ErrorLog("msg", "item was in the queue but the corresponding values does not exist as hash map")
		return nil
	}

	if err != nil {
		return err
	}

	if len(fns) == 0 {
		return nil
	}

	if err = c.incrementMapValues(source, fns); err != nil {
		return err
	}

	res, err := redisConn.Del(source)
	if err != redis.ErrNil {
		c.app.Logger.Log("msg", "we should be able to delete the counter here, but failed.", "err", err)
		return nil
	}

	if err != nil {
		return err
	}

	if res == 0 {
		c.app.Logger.Log("msg", "we should be able to delete the counter hash map here, but failed. someone might already have deleted it..")
	}

	return nil
}

func (c *compactorService) incrementMapValues(source string, fns map[string]int64) error {
	mongo := c.app.MustGetMongo()
	parsedKey := pkg.ParseKeyName(source)

	if parsedKey.Name == "" {
		return errors.New("name should be set")
	}

	return mongodb.InsertCompaction(
		mongo,
		parsedKey.Name,
		parsedKey.Direction,
		parsedKey.Segment,
		fns,
	)
}
