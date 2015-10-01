# go-chat-tcp

A simple chat client and server written in Go, implemented using TCP sockets.

### Setup

Start the server via `go run server.go` and then start as many clients as you want via `go run client.go`.

### Chat Commands

The following special chat commands exist: 

* `/help` lists all commands
* `/create foo` creates a chat room named foo
* `/list` lists all chat rooms
* `/join foo` joins a chat room named foo
* `/leave` leaves the current chat room
* `/name foo` changes the client name to foo
* `/quit` quits the program

Any other text is sent as a message to the current chat room.
