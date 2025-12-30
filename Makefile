build:
	go build -o p2p-transfer.exe ./cmd/p2p-transfer

server:
	go run ./cmd/p2p-transfer server --port 8000

# Start a default peer node
peer:
	go run ./cmd/p2p-transfer peer --addr 127.0.0.1:8001

# Start a peer and register a file (Seeder)
# Usage: make peer-seed FILE="path/to/file"
peer-seed:
	go run ./cmd/p2p-transfer peer --addr 127.0.0.1:8001 --register "$(FILE)"

# Start a peer and download a file (Leecher)
# Usage: make peer-leech HASH="file-hash-id"
peer-leech:
	go run ./cmd/p2p-transfer peer --addr 127.0.0.1:8002 --download "$(HASH)"