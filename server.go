package main

import (
	"strings"
	"fmt"
	"net"
	"os"
	"time"
	"bufio"
)

const (
	CONN_PORT = ":3333"
	CONN_TYPE = "tcp"

	CMD_PREFIX = "/"
	CMD_CREATE = CMD_PREFIX + "create"
	CMD_LIST   = CMD_PREFIX + "list"
	CMD_JOIN   = CMD_PREFIX + "join"
	CMD_LEAVE  = CMD_PREFIX + "leave"
	CMD_HELP   = CMD_PREFIX + "help"
	CMD_NAME   = CMD_PREFIX + "name"

	CLIENT_NAME = "Anonymous"
	SERVER_NAME = "Server"

	ERROR_PREFIX = "Error: "
	ERROR_SEND   = ERROR_PREFIX + "You cannot send messages in the lobby.\n"
	ERROR_CREATE = ERROR_PREFIX + "A chat room with that name already exists.\n"
	ERROR_JOIN   = ERROR_PREFIX + "A chat room with that name does not exist.\n"
	ERROR_LEAVE  = ERROR_PREFIX + "You cannot leave the lobby.\n"

	NOTICE_PREFIX = "Notice: "
	NOTICE_CREATE = NOTICE_PREFIX + "Created chat room \"%s\".\n"
	NOTICE_JOIN   = NOTICE_PREFIX + "\"%s\" joined the chat room.\n"
	NOTICE_LEAVE  = NOTICE_PREFIX + "\"%s\" left the chat room.\n"
	NOTICE_NAME   = NOTICE_PREFIX + "\"%s\" changed their name to \"%s\".\n"
	NOTICE_DELETE = NOTICE_PREFIX + "Chat room is inactive and being deleted.\n"


	EXPIRY_TIME time.Duration = 7 * 24 * time.Hour 
)

var serverClient *Client = &Client {
	name:     SERVER_NAME,
	incoming: nil,
	outgoing: nil,
	reader:   nil,
	writer:   nil,
}

type Lobby struct {
	clients     []*Client
	chatRooms   map[string]*ChatRoom
	incoming    chan *Message
	join        chan *Client
	delete      chan *ChatRoom
}

func NewLobby() *Lobby {
	lobby := &Lobby {
		clients:   make([]*Client, 0),
		chatRooms: make(map[string]*ChatRoom),
		incoming:  make(chan *Message),
		join:      make(chan *Client),
		delete:    make(chan *ChatRoom),
	}
	lobby.Listen()
	return lobby
}

func (lobby *Lobby) Listen() {
	go func() {
		for {
			select {
			case message := <-lobby.incoming:
				lobby.Parse(message)
			case client := <-lobby.join:
				lobby.Join(client)
			case chatRoom := <-lobby.delete:
				lobby.DeleteChatRoom(chatRoom)
			}
		}
	}()
}

func (lobby *Lobby) Join(client *Client) {
	lobby.clients = append(lobby.clients, client)
	go func() {
		for {
			message, ok := <-client.incoming
			if !ok {
				//signal to leave channel
			}
			lobby.incoming <- message
		} 
	}()
}

func (lobby *Lobby) Leave(client *Client) {
	//todo
}

func (lobby *Lobby) Parse(message *Message) {
	switch {
	default:
		lobby.SendMessage(message)
	case strings.HasPrefix(message.Text, CMD_CREATE):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_CREATE + " "), "\n")
		lobby.CreateChatRoom(message.Client, name)
	case strings.HasPrefix(message.Text, CMD_LIST):
		lobby.ListChatRooms(message.Client)
	case strings.HasPrefix(message.Text, CMD_JOIN):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_JOIN + " "), "\n")
		lobby.JoinChatRoom(message.Client, name)
	case strings.HasPrefix(message.Text, CMD_LEAVE):
		lobby.LeaveChatRoom(message.Client)
	case strings.HasPrefix(message.Text, CMD_NAME):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_NAME + " "), "\n")
		lobby.ChangeName(message.Client, name)
	case strings.HasPrefix(message.Text, CMD_HELP):
		lobby.Help(message.Client)
	}
}

func (lobby *Lobby) SendMessage(message *Message) {
	if message.Client.chatRoom == nil {
		message.Client.outgoing <- ERROR_SEND
		return
	}
	message.Client.chatRoom.Broadcast(message.String())
}

func (lobby *Lobby) CreateChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] != nil {
		client.outgoing <- ERROR_CREATE
		return
	}
	chatRoom := NewChatRoom(name)
	lobby.chatRooms[name] = chatRoom
	go func() {
		time.Sleep(EXPIRY_TIME)
		lobby.delete <- chatRoom
	}()
	client.outgoing <- fmt.Sprintf(NOTICE_CREATE, chatRoom.name)
}

func (lobby *Lobby) DeleteChatRoom(chatRoom *ChatRoom) {
	if chatRoom.expiry.After(time.Now()) {
		go func() {
			time.Sleep(chatRoom.expiry.Sub(time.Now()))
			lobby.delete <- chatRoom
		}()
	} else {
		chatRoom.Delete()
		delete(lobby.chatRooms, chatRoom.name)
	}
}

func (lobby *Lobby) JoinChatRoom(client *Client, name string) {
	if lobby.chatRooms[name] == nil {
		client.outgoing <- ERROR_JOIN
		return
	}
	if client.chatRoom != nil {
		lobby.LeaveChatRoom(client)
	}
	lobby.chatRooms[name].Join(client)
}

func (lobby *Lobby) LeaveChatRoom(client *Client) {
	if client.chatRoom == nil {
		client.outgoing <- ERROR_LEAVE
		return
	}
	client.chatRoom.Leave(client)
}

func (lobby *Lobby) ChangeName(client *Client, name string) {
	if client.chatRoom != nil {
		client.chatRoom.Broadcast(fmt.Sprintf(NOTICE_NAME, client.name, name))
	}
	client.name = name
}

func (lobby *Lobby) ListChatRooms(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Channels:\n"
	for name := range lobby.chatRooms {
		client.outgoing <- fmt.Sprintf("\"%s\"\n", name)
	}
	client.outgoing <- "\n"
}

func (lobby *Lobby) Help(client *Client) {
	client.outgoing <- "\n"
	client.outgoing <- "Commands:\n"
	client.outgoing <- "/help - lists all commands\n"
	client.outgoing <- "/list - lists all chat rooms\n"
	client.outgoing <- "/create foo - creates a chat room named foo\n"
	client.outgoing <- "/join foo - joins a chat room named foo\n"
	client.outgoing <- "/leave - leaves the current chat room\n"
	client.outgoing <- "/name foo - changes your name to foo\n"
	client.outgoing <- "\n"
}

type ChatRoom struct {
	name     string
	clients  []*Client
	messages []string
	expiry   time.Time
}

func NewChatRoom(name string) *ChatRoom {
	chatRoom := &ChatRoom {
		name:     name,
		clients:  make([]*Client, 0),
		messages: make([]string, 0),
		expiry:   time.Now().Add(EXPIRY_TIME),
	}
	return chatRoom
}

// Adds the given Client to the ChatRoom, and sends them all messages that have
// that have been sent since the creation of the ChatRoom.
func (chatRoom *ChatRoom) Join(client *Client) {
	client.chatRoom = chatRoom;
	for _, message := range chatRoom.messages {
		client.outgoing <- message
	}
	chatRoom.clients = append(chatRoom.clients, client)
	chatRoom.Broadcast(fmt.Sprintf(NOTICE_JOIN, client.name))
}

// Removes the given Client from the ChatRoom.
func (chatRoom *ChatRoom) Leave(client *Client) {
	chatRoom.Broadcast(fmt.Sprintf(NOTICE_LEAVE, client.name))
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

func (chatRoom *ChatRoom) Delete() {
	//notify of deletion?
	chatRoom.Broadcast(NOTICE_DELETE)
	for _, client := range chatRoom.clients {
		client.chatRoom = nil
	}
}

type Client struct {
	name     string
	chatRoom *ChatRoom
	incoming chan *Message
	outgoing chan string
	reader   *bufio.Reader
	writer   *bufio.Writer
}

// Returns a new client from the given connection, and starts a reader and
// writer which receive and send information from the socket
func NewClient(conn net.Conn) *Client {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	client := &Client {
		name: CLIENT_NAME,
		chatRoom: nil,
		incoming: make(chan *Message),
		outgoing: make(chan string),
		reader: reader,
		writer: writer,
	}

	go client.Read()
	go client.Write()

	return client
}

// Reads in strings from the Client's socket, formats them into Messages, and
// puts them into the Client's incoming channel.
func (client *Client) Read() {
	for {
		str, err := client.reader.ReadString('\n')
		if err != nil {
			fmt.Print(err)
			break
		}
		message := NewMessage(time.Now(), client, strings.TrimSuffix(str, "\n"))
		client.incoming <- message
	}
}

// Reads in messages from the Client's outgoing channel, and writes them to the
// Client's socket.
func (client *Client) Write() {
	for str := range client.outgoing {
		client.writer.WriteString(str)
		client.writer.Flush()
	}
}

type Message struct {
	Time   time.Time
	Client *Client 
	Text   string
}

func NewMessage(time time.Time, client *Client, text string) *Message {
	message := &Message {
		Time: time,
		Client: client,
		Text: text,
	}
	return message
}

func (message *Message) String() string {
	return fmt.Sprintf("%s - %s: %s\n", message.Time.Format(time.Kitchen), message.Client.name, message.Text)
}

func main() {
	lobby := NewLobby()

	// Listen for incoming connections.
	listener, err := net.Listen(CONN_TYPE, CONN_PORT)
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		os.Exit(1)
	}
		// Close the listener when the application closes.
	defer listener.Close()
	fmt.Println("Listening on " + CONN_PORT)
	for {
		// Listen for an incoming connection.
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			continue
		}
		// Handle connections in a new goroutine.
		go lobby.Join(NewClient(conn))
	}
}
