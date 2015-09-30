# golang-tcp-chat

A simple chat client and server written in Go, implemented using TCP sockets.

### Setup

Start the server via `go run server.go` and then start as many clients as you want via `go run client.go`.

### Chat Commands

The following special chat commands exist: 

* `/help` lists all commands
* `/create foo` creates a channel named foo
* `/list` lists all channels
* `/join foo` joins a channel named foo
* `/leave` leaves the current channel
* `/name foo` changes the client name to foo

Any other text is sent as a message to the current channel.
