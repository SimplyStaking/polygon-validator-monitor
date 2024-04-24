module monitor

go 1.20

require (
	github.com/ethereum/go-ethereum v1.13.8
	github.com/maticnetwork/heimdall v1.0.3
	github.com/mattn/go-sqlite3 v1.14.19
	github.com/prometheus/client_golang v1.18.0
)

require (
	github.com/BurntSushi/toml v1.2.1 // indirect
	github.com/DataDog/zstd v1.5.5 // indirect
	github.com/JekaMas/workerpool v1.1.8 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.3.2 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/cockroachdb/pebble v0.0.0-20231101195458-481da04154d6 // indirect
	github.com/deckarep/golang-set/v2 v2.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.2.0 // indirect
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/getsentry/sentry-go v0.25.0 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-stack/stack v1.8.1 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/holiman/uint256 v1.2.4 // indirect
	github.com/klauspost/compress v1.17.4 // indirect
	github.com/matttproud/golang_protobuf_extensions/v2 v2.0.0 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.45.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/rogpeppe/go-internal v1.11.0 // indirect
	github.com/shirou/gopsutil v3.21.4-0.20210419000835-c7a38de76ee5+incompatible // indirect
	github.com/syndtr/goleveldb v1.0.1-0.20220721030215-126854af5e6d // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	golang.org/x/crypto v0.17.0 // indirect
	golang.org/x/exp v0.0.0-20231110203233-9a3e6036ecaa // indirect
	golang.org/x/sync v0.5.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
	google.golang.org/protobuf v1.31.0 // indirect
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
)

replace github.com/tendermint/tendermint => github.com/maticnetwork/tendermint v0.26.0-dev0.0.20231005133805-2bb6a831bb2e

replace github.com/tendermint/tm-db => github.com/tendermint/tm-db v0.2.0

replace github.com/cosmos/cosmos-sdk => github.com/maticnetwork/cosmos-sdk v0.37.5-0.20231005133937-b1eb1f90feb7

replace github.com/ethereum/go-ethereum => github.com/maticnetwork/bor v1.0.4

replace go.mongodb.org/mongo-driver => go.mongodb.org/mongo-driver v1.5.1

replace github.com/libp2p/go-buffer-pool => github.com/libp2p/go-buffer-pool v0.1.0
