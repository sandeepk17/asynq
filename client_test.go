// Copyright 2020 Kentaro Hibino. All rights reserved.
// Use of this source code is governed by a MIT license
// that can be found in the LICENSE file.

package asynq

import (
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	h "github.com/hibiken/asynq/internal/asynqtest"
	"github.com/hibiken/asynq/internal/base"
)

func TestClient(t *testing.T) {
	r := setup(t)
	client := NewClient(&RedisClientOpt{
		Addr: "localhost:6379",
		DB:   14,
	})

	task := NewTask("send_email", map[string]interface{}{"to": "customer@gmail.com", "from": "merchant@example.com"})

	tests := []struct {
		desc          string
		task          *Task
		processAt     time.Time
		opts          []Option
		wantEnqueued  map[string][]*base.TaskMessage
		wantScheduled []h.ZSetEntry
	}{
		{
			desc:      "Process task immediately",
			task:      task,
			processAt: time.Now(),
			opts:      []Option{},
			wantEnqueued: map[string][]*base.TaskMessage{
				"default": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   defaultMaxRetry,
						Queue:   "default",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:         "Schedule task to be processed in the future",
			task:         task,
			processAt:    time.Now().Add(2 * time.Hour),
			opts:         []Option{},
			wantEnqueued: nil, // db is flushed in setup so list does not exist hence nil
			wantScheduled: []h.ZSetEntry{
				{
					Msg: &base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   defaultMaxRetry,
						Queue:   "default",
						Timeout: time.Duration(0).String(),
					},
					Score: float64(time.Now().Add(2 * time.Hour).Unix()),
				},
			},
		},
		{
			desc:      "Process task immediately with a custom retry count",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				MaxRetry(3),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"default": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   3,
						Queue:   "default",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:      "Negative retry count",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				MaxRetry(-2),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"default": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   0, // Retry count should be set to zero
						Queue:   "default",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:      "Conflicting options",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				MaxRetry(2),
				MaxRetry(10),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"default": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   10, // Last option takes precedence
						Queue:   "default",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:      "With queue option",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				Queue("custom"),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"custom": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   defaultMaxRetry,
						Queue:   "custom",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:      "Queue option should be case-insensitive",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				Queue("HIGH"),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"high": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   defaultMaxRetry,
						Queue:   "high",
						Timeout: time.Duration(0).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
		{
			desc:      "Timeout option sets the timeout duration",
			task:      task,
			processAt: time.Now(),
			opts: []Option{
				Timeout(20 * time.Second),
			},
			wantEnqueued: map[string][]*base.TaskMessage{
				"default": []*base.TaskMessage{
					&base.TaskMessage{
						Type:    task.Type,
						Payload: task.Payload.data,
						Retry:   defaultMaxRetry,
						Queue:   "default",
						Timeout: (20 * time.Second).String(),
					},
				},
			},
			wantScheduled: nil, // db is flushed in setup so zset does not exist hence nil
		},
	}

	for _, tc := range tests {
		h.FlushDB(t, r) // clean up db before each test case.

		err := client.Schedule(tc.task, tc.processAt, tc.opts...)
		if err != nil {
			t.Error(err)
			continue
		}

		for qname, want := range tc.wantEnqueued {
			gotEnqueued := h.GetEnqueuedMessages(t, r, qname)
			if diff := cmp.Diff(want, gotEnqueued, h.IgnoreIDOpt); diff != "" {
				t.Errorf("%s;\nmismatch found in %q; (-want,+got)\n%s", tc.desc, base.QueueKey(qname), diff)
			}
		}

		gotScheduled := h.GetScheduledEntries(t, r)
		if diff := cmp.Diff(tc.wantScheduled, gotScheduled, h.IgnoreIDOpt); diff != "" {
			t.Errorf("%s;\nmismatch found in %q; (-want,+got)\n%s", tc.desc, base.ScheduledQueue, diff)
		}
	}
}
