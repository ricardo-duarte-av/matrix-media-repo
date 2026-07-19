package redislib

import (
	"context"
	"sync"

	"github.com/redis/go-redis/v9"
	"github.com/t2bot/matrix-media-repo/common/rcontext"
)

var subscribeMutex = new(sync.Mutex)
var subscribeChans = make(map[string][]chan string)

// activeSubs tracks the live redis subscriptions so they can be explicitly closed
// on reconnect/stop. Closing the ring does not reliably terminate a PubSub.Channel()
// reader goroutine (go-redis retries internally), so without this each Reconnect would
// leak the previous subscriptions' goroutines. Guarded by subscribeMutex.
var activeSubs = make([]*redis.PubSub, 0)

type PubSubValue struct {
	Err error
	Str string
}

func Publish(ctx rcontext.RequestContext, channel string, payload string) error {
	makeConnection()
	if ring == nil {
		return nil
	}

	if ring.PoolStats().TotalConns == 0 {
		ctx.Log.Warn("Not broadcasting upload to Redis - no connections available")
		return nil
	}

	r := ring.Publish(ctx.Context, channel, payload)
	if r.Err() != nil {
		if r.Err() == redis.Nil {
			ctx.Log.Warn("Not broadcasting upload to Redis - no connections available")
			return nil
		}
		return r.Err()
	}
	return nil
}

func Subscribe(channel string) <-chan string {
	makeConnection()
	if ring == nil {
		return nil
	}

	// Buffered so a momentarily-slow consumer doesn't block the reader goroutine below.
	ch := make(chan string, 128)
	subscribeMutex.Lock()
	defer subscribeMutex.Unlock()
	if _, ok := subscribeChans[channel]; !ok {
		subscribeChans[channel] = make([]chan string, 0)
	}
	subscribeChans[channel] = append(subscribeChans[channel], ch)
	doSubscribe(channel, ch)
	return ch
}

// doSubscribe must be called with subscribeMutex held.
func doSubscribe(channel string, ch chan<- string) {
	sub := ring.Subscribe(context.Background(), channel)
	activeSubs = append(activeSubs, sub)
	go func(ch chan<- string) {
		// Ranging exits cleanly when sub.Close() closes the channel (see
		// resubscribeAll), so this goroutine terminates instead of leaking.
		for val := range sub.Channel() {
			// Non-blocking send: if the consumer isn't keeping up we drop the
			// message rather than wedge this goroutine forever holding it.
			select {
			case ch <- val.Payload:
			default:
			}
		}
	}(ch)
}

func resubscribeAll() {
	subscribeMutex.Lock()
	defer subscribeMutex.Unlock()

	// Terminate the previous subscriptions' reader goroutines before creating new ones.
	for _, sub := range activeSubs {
		_ = sub.Close()
	}
	activeSubs = make([]*redis.PubSub, 0)

	for channel, chs := range subscribeChans {
		for _, ch := range chs {
			if ring == nil {
				close(ch)
			} else {
				doSubscribe(channel, ch)
			}
		}
	}
	if ring == nil {
		subscribeChans = make(map[string][]chan string)
	}
}
