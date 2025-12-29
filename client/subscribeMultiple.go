package client

type MultiState[T any] struct {
	Data    []Meta[T]
	Updated bool
}

// SubscribeMultipleList2Events holds event callbacks for SubscribeMultipleList2.
// Required: OnMessage.
// Optional: OnError - receives errors with nil for connections without errors.
type SubscribeMultipleList2Events[T1, T2 any] struct {
	OnMessage func(MultiState[T1], MultiState[T2])
	OnError   func(err1, err2 error)
}

// SubscribeMultipleList3Events holds event callbacks for SubscribeMultipleList3.
// Required: OnMessage.
// Optional: OnError - receives errors with nil for connections without errors.
type SubscribeMultipleList3Events[T1, T2, T3 any] struct {
	OnMessage func(MultiState[T1], MultiState[T2], MultiState[T3])
	OnError   func(err1, err2, err3 error)
}

// SubscribeMultipleList4Events holds event callbacks for SubscribeMultipleList4.
// Required: OnMessage.
// Optional: OnError - receives errors with nil for connections without errors.
type SubscribeMultipleList4Events[T1, T2, T3, T4 any] struct {
	OnMessage func(MultiState[T1], MultiState[T2], MultiState[T3], MultiState[T4])
	OnError   func(err1, err2, err3, err4 error)
}

// SubscribeMultipleList2 subscribes to 2 list paths (glob patterns) with different types and a single callback.
// When any subscription updates, the callback receives ALL current states.
// Uses typed channels for type-safe, lock-free state management.
// cfg provides the shared connection configuration (Server, Header, HandshakeTimeout, Retry, Silence).
// paths is an array of 2 glob pattern paths to subscribe to.
func SubscribeMultipleList2[T1, T2 any](
	cfg SubscribeConfig,
	paths [2]string,
	events SubscribeMultipleList2Events[T1, T2],
) {
	ch1 := make(chan []Meta[T1], 10)
	ch2 := make(chan []Meta[T2], 10)

	// State manager goroutine - single point of state mutation
	go func() {
		var state1 []Meta[T1]
		var state2 []Meta[T2]

		for {
			select {
			case <-cfg.Ctx.Done():
				return
			case state1 = <-ch1:
				events.OnMessage(MultiState[T1]{Data: state1, Updated: true}, MultiState[T2]{Data: state2, Updated: false})
			case state2 = <-ch2:
				events.OnMessage(MultiState[T1]{Data: state1, Updated: false}, MultiState[T2]{Data: state2, Updated: true})
			}
		}
	}()

	var onError1 func(error)
	var onError2 func(error)
	if events.OnError != nil {
		onError1 = func(err error) { events.OnError(err, nil) }
		onError2 = func(err error) { events.OnError(nil, err) }
	}

	go SubscribeList(cfg, paths[0], SubscribeListEvents[T1]{
		OnMessage: func(messages []Meta[T1]) {
			select {
			case ch1 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError1,
	})

	go SubscribeList(cfg, paths[1], SubscribeListEvents[T2]{
		OnMessage: func(messages []Meta[T2]) {
			select {
			case ch2 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError2,
	})
}

// SubscribeMultipleList3 subscribes to 3 list paths (glob patterns) with different types and a single callback.
// When any subscription updates, the callback receives ALL current states.
// Uses typed channels for type-safe, lock-free state management.
// cfg provides the shared connection configuration (Server, Header, HandshakeTimeout, Retry, Silence).
// paths is an array of 3 glob pattern paths to subscribe to.
func SubscribeMultipleList3[T1, T2, T3 any](
	cfg SubscribeConfig,
	paths [3]string,
	events SubscribeMultipleList3Events[T1, T2, T3],
) {
	ch1 := make(chan []Meta[T1], 10)
	ch2 := make(chan []Meta[T2], 10)
	ch3 := make(chan []Meta[T3], 10)

	// State manager goroutine - single point of state mutation
	go func() {
		var state1 []Meta[T1]
		var state2 []Meta[T2]
		var state3 []Meta[T3]

		for {
			select {
			case <-cfg.Ctx.Done():
				return
			case state1 = <-ch1:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: true},
					MultiState[T2]{Data: state2, Updated: false},
					MultiState[T3]{Data: state3, Updated: false},
				)
			case state2 = <-ch2:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: false},
					MultiState[T2]{Data: state2, Updated: true},
					MultiState[T3]{Data: state3, Updated: false},
				)
			case state3 = <-ch3:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: false},
					MultiState[T2]{Data: state2, Updated: false},
					MultiState[T3]{Data: state3, Updated: true},
				)
			}
		}
	}()

	var onError1 func(error)
	var onError2 func(error)
	var onError3 func(error)
	if events.OnError != nil {
		onError1 = func(err error) { events.OnError(err, nil, nil) }
		onError2 = func(err error) { events.OnError(nil, err, nil) }
		onError3 = func(err error) { events.OnError(nil, nil, err) }
	}

	go SubscribeList(cfg, paths[0], SubscribeListEvents[T1]{
		OnMessage: func(messages []Meta[T1]) {
			select {
			case ch1 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError1,
	})

	go SubscribeList(cfg, paths[1], SubscribeListEvents[T2]{
		OnMessage: func(messages []Meta[T2]) {
			select {
			case ch2 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError2,
	})

	go SubscribeList(cfg, paths[2], SubscribeListEvents[T3]{
		OnMessage: func(messages []Meta[T3]) {
			select {
			case ch3 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError3,
	})
}

// SubscribeMultipleList4 subscribes to 4 list paths (glob patterns) with different types and a single callback.
// When any subscription updates, the callback receives ALL current states.
// Uses typed channels for type-safe, lock-free state management.
// cfg provides the shared connection configuration (Server, Header, HandshakeTimeout, Retry, Silence).
// paths is an array of 4 glob pattern paths to subscribe to.
func SubscribeMultipleList4[T1, T2, T3, T4 any](
	cfg SubscribeConfig,
	paths [4]string,
	events SubscribeMultipleList4Events[T1, T2, T3, T4],
) {
	ch1 := make(chan []Meta[T1], 10)
	ch2 := make(chan []Meta[T2], 10)
	ch3 := make(chan []Meta[T3], 10)
	ch4 := make(chan []Meta[T4], 10)

	// State manager goroutine - single point of state mutation
	go func() {
		var state1 []Meta[T1]
		var state2 []Meta[T2]
		var state3 []Meta[T3]
		var state4 []Meta[T4]

		for {
			select {
			case <-cfg.Ctx.Done():
				return
			case state1 = <-ch1:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: true},
					MultiState[T2]{Data: state2, Updated: false},
					MultiState[T3]{Data: state3, Updated: false},
					MultiState[T4]{Data: state4, Updated: false},
				)
			case state2 = <-ch2:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: false},
					MultiState[T2]{Data: state2, Updated: true},
					MultiState[T3]{Data: state3, Updated: false},
					MultiState[T4]{Data: state4, Updated: false},
				)
			case state3 = <-ch3:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: false},
					MultiState[T2]{Data: state2, Updated: false},
					MultiState[T3]{Data: state3, Updated: true},
					MultiState[T4]{Data: state4, Updated: false},
				)
			case state4 = <-ch4:
				events.OnMessage(
					MultiState[T1]{Data: state1, Updated: false},
					MultiState[T2]{Data: state2, Updated: false},
					MultiState[T3]{Data: state3, Updated: false},
					MultiState[T4]{Data: state4, Updated: true},
				)
			}
		}
	}()

	var onError1 func(error)
	var onError2 func(error)
	var onError3 func(error)
	var onError4 func(error)
	if events.OnError != nil {
		onError1 = func(err error) { events.OnError(err, nil, nil, nil) }
		onError2 = func(err error) { events.OnError(nil, err, nil, nil) }
		onError3 = func(err error) { events.OnError(nil, nil, err, nil) }
		onError4 = func(err error) { events.OnError(nil, nil, nil, err) }
	}

	go SubscribeList(cfg, paths[0], SubscribeListEvents[T1]{
		OnMessage: func(messages []Meta[T1]) {
			select {
			case ch1 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError1,
	})

	go SubscribeList(cfg, paths[1], SubscribeListEvents[T2]{
		OnMessage: func(messages []Meta[T2]) {
			select {
			case ch2 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError2,
	})

	go SubscribeList(cfg, paths[2], SubscribeListEvents[T3]{
		OnMessage: func(messages []Meta[T3]) {
			select {
			case ch3 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError3,
	})

	go SubscribeList(cfg, paths[3], SubscribeListEvents[T4]{
		OnMessage: func(messages []Meta[T4]) {
			select {
			case ch4 <- messages:
			case <-cfg.Ctx.Done():
			}
		},
		OnError: onError4,
	})
}
