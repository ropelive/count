package compactor

import (
	"flag"
	"math/rand"
	"strconv"
	"testing"
	"time"

	redigo "github.com/garyburd/redigo/redis"
	"github.com/koding/redis"
	"github.com/koding/ropecount/pkg"
)

func withApp(fn func(app *pkg.App)) {
	name := "compator_test"
	conf := flag.CommandLine // tests add more stuff

	pkg.AddHTTPConf(conf)
	pkg.AddRedisConf(conf)

	app := pkg.NewApp(name, conf)
	fn(app)
}

func Test_compactorService_incrementMapValues(t *testing.T) {
	withApp(func(app *pkg.App) {
		var redisConn *redis.RedisSession
		{
			redisConn = app.MustGetRedis()
			rand.Seed(time.Now().UnixNano())
			prefix := strconv.Itoa(rand.Int())
			redisConn.SetPrefix(prefix)
		}

		type fields struct {
			app *pkg.App
		}
		type args struct {
			redisConn *redis.RedisSession
			target    string
			fns       map[string]int64
		}
		tests := []struct {
			name    string
			fields  fields
			args    args
			wantErr bool
		}{
			{
				name: "bare call",
				fields: fields{
					app: app,
				},
				args: args{
					redisConn: redisConn,
					target:    "my_test_hash_bare_call",
					fns: map[string]int64{
						"key1": 1,
						"key2": 2,
						"key3": 3,
					},
				},
				wantErr: false,
			},
			{
				name: "empty call",
				fields: fields{
					app: app,
				},
				args: args{
					redisConn: redisConn,
					target:    "my_test_hash_empty_call",
					fns:       map[string]int64{},
				},
				wantErr: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				c := &compactorService{
					app: tt.fields.app,
				}
				if err := c.incrementMapValues(tt.args.redisConn, tt.args.target, tt.args.fns); (err != nil) != tt.wantErr {
					t.Errorf("compactorService.incrementMapValues() error = %v, wantErr %v", err, tt.wantErr)
				}

				fns, err := redigo.Int64Map(redisConn.HashGetAll(tt.args.target))
				if err != nil {
					t.Errorf("redigo.Int64Map(redisConn.HashGetAll(tt.args.target)) error = %v", err)
				}

				if len(fns) != len(tt.args.fns) {
					t.Errorf("len(fns) %d != len(tt.args.fns) %d", len(fns), len(tt.args.fns))
				}

				for key, val := range tt.args.fns {
					if fns[key] != val {
						t.Errorf(" fns[key] != val | %d != %d", fns[key], val)
					}
				}

				if _, err := redisConn.Del(tt.args.target); err != nil {
					t.Errorf("redisConn.Del(tt.args.target) error = %v", err)
				}
			})
		}
	})
}

func Test_compactorService_merge(t *testing.T) {
	withApp(func(app *pkg.App) {
		var redisConn *redis.RedisSession
		{
			redisConn = app.MustGetRedis()
			rand.Seed(time.Now().UnixNano())
			prefix := strconv.Itoa(rand.Int())
			redisConn.SetPrefix(prefix)
		}

		type fields struct {
			app *pkg.App
		}
		type args struct {
			redisConn  *redis.RedisSession
			source     string
			sourceVals map[string]interface{}
			target     string
			targetVals map[string]interface{}
		}
		tests := []struct {
			name    string
			fields  fields
			args    args
			result  map[string]int64
			wantErr bool
		}{
			{
				name: "empty call",
				fields: fields{
					app: app,
				},
				args: args{
					redisConn: redisConn,
					source:    "my_source",
					sourceVals: map[string]interface{}{
						"key1": 1,
						"key2": 2,
					},
					target: "my_target",
					targetVals: map[string]interface{}{
						"key2": 2,
						"key3": 3,
					},
				},
				result: map[string]int64{
					"key1": 1,
					"key2": 4,
					"key3": 3,
				},
				wantErr: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				c := &compactorService{
					app: tt.fields.app,
				}
				if err := redisConn.HashMultipleSet(tt.args.source, tt.args.sourceVals); err != nil {
					t.Errorf("redisConn.HashMultipleSet(tt.args.source, tt.args.sourceVals) error = %v, wantErr %v", err, tt.wantErr)
				}

				if err := redisConn.HashMultipleSet(tt.args.target, tt.args.targetVals); err != nil {
					t.Errorf("redisConn.HashMultipleSet(tt.args.source, tt.args.sourceVals) error = %v, wantErr %v", err, tt.wantErr)
				}

				if err := c.merge(tt.args.redisConn, tt.args.source, tt.args.target); (err != nil) != tt.wantErr {
					t.Errorf("compactorService.merge() error = %v, wantErr %v", err, tt.wantErr)
				}
				fns, err := redigo.Int64Map(redisConn.HashGetAll(tt.args.target))
				if err != nil {
					t.Errorf("redigo.Int64Map(redisConn.HashGetAll(tt.args.target)) error = %v", err)
				}

				if len(fns) != len(tt.result) {
					t.Errorf("len(fns) %d != len(tt.result) %d", len(fns), len(tt.result))
				}

				for key, val := range tt.result {
					if fns[key] != val {
						t.Errorf(" fns[key] != val | %d != %d", fns[key], val)
					}
				}

				if _, err := redisConn.Del(tt.args.target); err != nil {
					t.Errorf("redisConn.Del(tt.args.target) error = %v", err)
				}
				if _, err := redisConn.Del(tt.args.source); err != nil {
					t.Errorf("redisConn.Del(tt.args.source) error = %v", err)
				}
			})
		}
	})
}
