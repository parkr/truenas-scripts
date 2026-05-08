# truenas_scripts

Scripts used to manage my TrueNAS Scale 25.10.x instance running on a
UGREEN DXP4800 Plus with 64GB of RAM, 4x12TB HDD in pool Tank, and 2x4TB
NVME SSD in pool Flash.

## API Documentation

Documentation can be found here: https://api.truenas.com/v25.10/jsonrpc.html

It uses JSON-RPC 2.0 Protocol over websockets.

## Best Practices

- Write all scripts in Golang, using the
  github.com/truenas/api_client_golang websocket client where possible.
- Write tests for all scripts unless the script is written in bash.
- Structure this repo as you would a Golang mono repo: commands in
  `cmd/<command_name>/main.go`, with shared code in `<module_name/` in the
  root of this directory. Use Go modules with `go.mod` and `go.sum` in the
  root of the directory.
- Always run `go test ./...` to ensure all changes pass test.
