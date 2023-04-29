module github.com/brianmcgee/enats

go 1.20

require (
	github.com/alecthomas/kong v0.7.1
	github.com/emirpasic/gods v1.18.1
	github.com/ethereum/go-ethereum v1.11.6
	github.com/juju/errors v1.0.0
	github.com/nats-io/nats.go v1.25.0
	gitlab.com/pjrpc/pjrpc/v2 v2.3.1
	go.etcd.io/bbolt v1.3.7
	go.uber.org/zap v1.24.0
	golang.org/x/net v0.9.0
	golang.org/x/sync v0.1.0
)

require (
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/holiman/uint256 v1.2.2-0.20230321075855-87b91420868c // indirect
	github.com/nats-io/nats-server/v2 v2.9.16 // indirect
	github.com/nats-io/nkeys v0.4.4 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/puzpuzpuz/xsync/v2 v2.4.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	golang.org/x/crypto v0.8.0 // indirect
	golang.org/x/sys v0.7.0 // indirect
)

replace gitlab.com/pjrpc/pjrpc/v2 => gitlab.com/bnmcgee/pjrpc/v2 v2.3.2-0.20230427113152-c98ee0ec568a
