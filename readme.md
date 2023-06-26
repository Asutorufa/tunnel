# tunnel

A simple NAT traversal tools, current only support TCP.

## build

```shell
make linux
make windows
make darwin
```

## server

```shell
server -h 127.0.0.1:8388
```

## client

```shell
client -s private.server.com:8388 -uuid uuid -s5 127.0.0.1:1080 -r rule.json
```

rule.json

```json
{
    "127.0.0.1:56022": { // local listen address
        "uuid": "uuid1", // target device uuid
        "address": "127.0.0.1", // target address
        "port": 50051 // target port
    },
    "127.0.0.1:56023": {
        "uuid": "uuid2",
        "address": "127.0.0.1",
        "port": 8000
    },
    "127.0.0.1:56024": {
        "uuid": "uuid3",
        "address": "127.0.0.1",
        "port": 22
    }
}
```
