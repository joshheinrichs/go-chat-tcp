package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

const (
	CONN_PORT = ":3333"
	CONN_TYPE = "tcp"

	MAX_CLIENTS = 10

	CMD_PREFIX = "/"
	CMD_CREATE = CMD_PREFIX + "create"
	CMD_LIST   = CMD_PREFIX + "list"
	CMD_JOIN   = CMD_PREFIX + "join"
	CMD_LEAVE  = CMD_PREFIX + "leave"
	CMD_HELP   = CMD_PREFIX + "help"
	CMD_NAME   = CMD_PREFIX + "name"
	CMD_QUIT   = CMD_PREFIX + "quit"

	CLIENT_NAME = "Anonymous"
	SERVER_NAME = "Server"

	ERROR_PREFIX = "Error: "
	ERROR_SEND   = ERROR_PREFIX + "You cannot send messages in the lobby.\n"
	ERROR_CREATE = ERROR_PREFIX + "A chat room with that name already exists.\n"
	ERROR_JOIN   = ERROR_PREFIX + "A chat room with that name does not exist.\n"
	ERROR_LEAVE  = ERROR_PREFIX + "You cannot leave the lobby.\n"

	NOTICE_PREFIX          = "Notice: "
	NOTICE_ROOM_JOIN       = NOTICE_PREFIX + "\"%s\" joined the chat room.\n"
	NOTICE_ROOM_LEAVE      = NOTICE_PREFIX + "\"%s\" left the chat room.\n"
	NOTICE_ROOM_NAME       = NOTICE_PREFIX + "\"%s\" changed their name to \"%s\".\n"
	NOTICE_ROOM_DELETE     = NOTICE_PREFIX + "Chat room is inactive and being deleted.\n"
	NOTICE_PERSONAL_CREATE = NOTICE_PREFIX + "Created chat room \"%s\".\n"
	NOTICE_PERSONAL_NAME   = NOTICE_PREFIX + "Changed name to \"\".\n"

	MSG_CONNECT = "Welcome to the server! Type \"/help\" to get a list of commands.\n"
	MSG_FULL    = "Server is full. Please try reconnecting later."

	EXPIRY_TIME time.Duration = 7 * 24 * time.Hour
)

// A Lobby receives messages on its channels, and keeps track of the currently
// connected clients, and currently created chat rooms.
type Lobby struct {
	clients   []*Client
	chatRooms map[string]*ChatRoom
	incoming  chan *Message
	join      chan *Client
	leave     chan *Client
	delete    chan *ChatRoom
}

// Creates a lobby which beings listening over its channels.
func NewLobby() *Lobby {
	lobby := &Lobby{
		clients:   make([]*Client, 0),
		chatRooms: make(map[string]*ChatRoom),
		incoming:  make(chan *Message),
		join:      make(chan *Client),
		leave:     make(chan *Client),
		delete:    make(chan *ChatRoom),
	}
	lobby.Listen()
	return lobby
}

// Starts a new thread which listens over the Lobby's various channels.
func (lobby *Lobby) Listen() {
	go func() {
		for {
			select {
			case message := <-lobby.incoming:
				lobby.Parse(message)
			case client := <-lobby.join:
				lobby.Join(client)
			case client := <-lobby.leave:
				lobby.Leave(client)
			case chatRoom := <-lobby.delete:
				lobby.DeleteChatRoom(chatRoom)
			}
		}
	}()
}

// Handles clients connecting to the lobby
func (lobby *Lobby) Join(client *Client) {
	if len(lobby.clients) >= MAX_CLIENTS {
		client.Quit()
		return
	}
	lobby.clients = append(lobby.clients, client)
	client.outgoing <- MSG_CONNECT
	go func() {
		for message := range client.incoming {
			lobby.incoming <- message
		}
		lobby.leave <- client
	}()
}

// Handles clients disconnecting from the lobby.
func (lobby *Lobby) Leave(client *Client) {
	if client.chatRoom != nil {
		client.chatRoom.Leave(client)
	}
	for i, otherClient := range lobby.clients {
		if client == otherClient {
			lobby.clients = append(lobby.clients[:i], lobby.clients[i+1:]...)
			break
		}
	}
	close(client.outgoing)
	log.Println("Closed client's outgoing channel")
}

// Checks if the a channel has expired. If it has, the chat room is deleted.
// Otherwise, a signal is sent to the delete channel at its new expiry time.
func (lobby *Lobby) DeleteChatRoom(chatRoom *ChatRoom) {
	if chatRoom.expiry.After(time.Now()) {
		go func() {
			time.Sleep(chatRoom.expiry.Sub(time.Now()))
			lobby.delete <- chatRoom
		}()
		log.Println("attempted to delete chat room")
	} else {
		chatRoom.Delete()
		delete(lobby.chatRooms, chatRoom.name)
		log.Println("deleted chat room")
	}
}

// Handles messages sent to the lobby. If the message contains a command, the
// command is executed by the lobby. Otherwise, the message is sent to the
// sender's current chat room.
func (lobby *Lobby) Parse(message *Message) {
	switch {
	default:
		lobby.SendMessage(message)
	case strings.HasPrefix(message.text, CMD_CREATE):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CMD_CREATE+" "), "\n")
		lobby.CreateChatRoom(message.client, name)
	case strings.HasPrefix(message.text, CMD_LIST):
		lobby.ListChatRooms(message.client)
	case strings.HasPrefix(message.text, CMD_JOIN):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CMD_JOIN+" "), "\n")
		lobby.JoinChatRoom(message.client, name)
	case strings.HasPrefix(message.text, CMD_LEAVE):
		lobby.LeaveChatRoom(message.client)
	case strings.HasPrefix(message.text, CMD_NAME):
		name := strings.TrimSuffix(strings.TrimPrefix(message.text, CMD_NAME+" "), "\n")
		lobby.ChangeName(message.client, name)
	case strings.HasPrefix(message.text, CMD_HELP):
		lobby.Help(message.client)
	case strings.HasPrefix(message.text, CMD_QUIT):
		message.client.Quit()
	}
}

// Attempts to send the given message to the client's current chat room. If they
// are not in a chat room, an error message is sent to the client.
func (lobby *Lobby) SendMessage(message *Message) {
	if message.client.chatRoom == nil {
		message.client.outgoing <- ERROR_SEND
		log.Println("client tried to send message in lobby")
		return
	}
	message.client.chatRoom.Broadcast(message.String())
	log.Println("client sent message")
}

// Attempts to create a chat room with the given name, provided that one does
// not already exist.
func (lobby *Lobby) CreateChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] != nil {
		client.outgoing <- ERROR_CREATE
		log.Println("client tried to create chat room with a name already in use")
		return
	}
	chatRoom := NewChatRoom(name)
	lobby.chatRooms[name] = chatRoom
	go func() {
		time.Sleep(EXPIRY_TIME)
		lobby.delete <- chatRoom
	}()
	client.outgoing <- fmt.Sprintf(NOTICE_PERSONAL_CREATE, chatRoom.name)
	log.Println("client created chat room")
}

// Attempts to add the client to the chat room with the given name, provided
// that the chat room exists.
func (lobby *Lobby) JoinChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] == nil {
		client.outgoing <- ERROR_JOIN
		log.Println("client tried to join a chat room that does not exist")
		return
	}
	if client.chatRoom != nil {
		lobby.LeaveChatRoom(client)
	}
	lobby.chatRooms[name].Join(client)
	log.Println("client joined chat room")
}

// Removes the given client from their current chat room.
func (lobby *Lobby) LeaveChatRoom(client *Client) {
	if client.chatRoom == nil {
		client.outgoing <- ERROR_LEAVE
		log.Println("client tried to leave the lobby")
		return
	}
	client.chatRoom.Leave(client)
	log.Println("client left chat room")
}

// Changes the client's name to the given name.
func (lobby *Lobby) ChangeName(client *Client, name string) {
	if client.chatRoom == nil {
		client.outgoing <- fmt.Sprintf(NOTICE_PERSONAL_NAME, name)
	} else {
		client.chatRoom.Broadcast(fmt.Sprintf(NOTICE_ROOM_NAME, client.name, name))
	}
	client.name = name
	log.Println("client changed their name")
}

// Sends to the client the list of chat rooms currently open.
func (lobby *Lobby) ListChatRooms(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Chat Rooms:\n"
	for name := range lobby.chatRooms {
		client.outgoing <- fmt.Sprintf("%s\n", name)
	}
	client.outgoing <- "\n"
	log.Println("client listed chat rooms")
}

// Sends to the client the list of possible commands to the client.
func (lobby *Lobby) Help(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Commands:\n"
	client.outgoing <- "/help - lists all commands\n"
	client.outgoing <- "/list - lists all chat rooms\n"
	client.outgoing <- "/create foo - creates a chat room named foo\n"
	client.outgoing <- "/join foo - joins a chat room named foo\n"
	client.outgoing <- "/leave - leaves the current chat room\n"
	client.outgoing <- "/name foo - changes your name to foo\n"
	client.outgoing <- "/quit - quits the program\n"
	client.outgoing <- "\n"
	log.Println("client requested help")
}

// A ChatRoom contains the chat's name, a list of the currently connected
// clients, a history of the messages broadcast to the users in the channel,
// and the current time at which the ChatRoom will expire.
type ChatRoom struct {
	name     string
	clients  []*Client
	messages []string
	expiry   time.Time
}

// Creates an empty chat room with the given name, and sets its expiry time to
// the current time + EXPIRY_TIME.
func NewChatRoom(name string) *ChatRoom {
	return &ChatRoom{
		name:     name,
		clients:  make([]*Client, 0),
		messages: make([]string, 0),
		expiry:   time.Now().Add(EXPIRY_TIME),
	}
}

// Adds the given Client to the ChatRoom, and sends them all messages that have
// that have been sent since the creation of the ChatRoom.
func (chatRoom *ChatRoom) Join(client *Client) {
	client.chatRoom = chatRoom
	for _, message := range chatRoom.messages {
		client.outgoing <- message
	}
	chatRoom.clients = append(chatRoom.clients, client)
	chatRoom.Broadcast(fmt.Sprintf(NOTICE_ROOM_JOIN, client.name))
}

// Removes the given Client from the ChatRoom.
func (chatRoom *ChatRoom) Leave(client *Client) {
	chatRoom.Broadcast(fmt.Sprintf(NOTICE_ROOM_LEAVE, client.name))
	for i, otherClient := range chatRoom.clients {
		if client == otherClient {
			chatRoom.clients = append(chatRoom.clients[:i], chatRoom.clients[i+1:]...)
			break
		}
	}
	client.chatRoom = nil
}

// Sends the given message to all Clients currently in the ChatRoom.
func (chatRoom *ChatRoom) Broadcast(message string) {
	chatRoom.expiry = time.Now().Add(EXPIRY_TIME)
	chatRoom.messages = append(chatRoom.messages, message)
	for _, client := range chatRoom.clients {
		client.outgoing <- message
	}
}

// Notifies the clients within the chat room that it is being deleted, and kicks
// them back into the lobby.
func (chatRoom *ChatRoom) Delete() {
	//notify of deletion?
	chatRoom.Broadcast(NOTICE_ROOM_DELETE)
	for _, client := range chatRoom.clients {
		client.chatRoom = nil
	}
}

// A client abstracts away the idea of a connection into incoming and outgoing
// channels, and stores some information about the client's state, including
// their current name and chat room.
type Client struct {
	name     string
	chatRoom *ChatRoom
	incoming chan *Message
	outgoing chan string
	conn     net.Conn
	reader   *bufio.Reader
	writer   *bufio.Writer
}

// Returns a new client from the given connection, and starts a reader and
// writer which receive and send information from the socket
func NewClient(conn net.Conn) *Client {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	client := &Client{
		name:     CLIENT_NAME,
		chatRoom: nil,
		incoming: make(chan *Message),
		outgoing: make(chan string),
		conn:     conn,
		reader:   reader,
		writer:   writer,
	}

	client.Listen()
	return client
}

// Starts two threads which read from the client's outgoing channel and write to
// the client's socket connection, and read from the client's socket and write
// to the client's incoming channel.
func (client *Client) Listen() {
	go client.Read()
	go client.Write()
}

// Reads in strings from the Client's socket, formats them into Messages, and
// puts them into the Client's incoming channel.
func (client *Client) Read() {
	for {
		str, err := client.reader.ReadString('\n')
		if err != nil {
			log.Println(err)
			break
		}
		message := NewMessage(time.Now(), client, strings.TrimSuffix(str, "\n"))
		client.incoming <- message
	}
	close(client.incoming)
	log.Println("Closed client's incoming channel read thread")
}

// Reads in messages from the Client's outgoing channel, and writes them to the
// Client's socket.
func (client *Client) Write() {
	for str := range client.outgoing {
		_, err := client.writer.WriteString(str)
		if err != nil {
			log.Println(err)
			break
		}
		err = client.writer.Flush()
		if err != nil {
			log.Println(err)
			break
		}
	}
	log.Println("Closed client's write thread")
}

// Closes the client's connection. Socket closing is by error checking, so this
// takes advantage of that to simplify the code and make sure all the threads
// are cleaned up.
func (client *Client) Quit() {
	client.conn.Close()
}

// A Message contains information about the sender, the time at which the
// message was sent, and the text of the message. This gives a convenient way
// of passing the necessary information about a message from the client to the
// lobby.
type Message struct {
	time   time.Time
	client *Client
	text   string
}

// Creates a new message with the given time, client and text.
func NewMessage(time time.Time, client *Client, text string) *Message {
	return &Message{
		time:   time,
		client: client,
		text:   text,
	}
}

// Returns a string representation of the message.
func (message *Message) String() string {
	return fmt.Sprintf("%s - %s: %s\n", message.time.Format(time.Kitchen), message.client.name, message.text)
}

// Creates a lobby, listens for client connections, and connects them to the
// lobby.
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	lobby := NewLobby()

	listener, err := net.Listen(CONN_TYPE, CONN_PORT)
	if err != nil {
		log.Println("Error: ", err)
		os.Exit(1)
	}
	defer listener.Close()
	log.Println("Listening on " + CONN_PORT)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error: ", err)
			continue
		}
		lobby.Join(NewClient(conn))
	}
}
