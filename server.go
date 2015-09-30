package main

import (
	"strings"
	"fmt"
	"net"
	"os"
	"time"
	"bufio"
	"errors"
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
	ERROR_SEND   = ERORR_PREFIX + "You cannot send messages in the lobby"
	ERROR_CREATE = ERROR_PREFIX + "A chat room with that name already exists."
	ERROR_JOIN   = ERROR_PREFIX + "A chat room with that name does not exist."
	ERROR_LEAVE  = ERROR_PREFIX + "You cannot leave the lobby."
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
				lobby.RemoveChatRoom(chatRoom)
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
		fmt.Printf("%s", message)
		if message.Client.chatRoom == nil {
			serverMessage := NewMessage(time.Now(), serverClient, ERROR_SEND)
			message.Client.outgoing <- serverMessage.String()
			return
		}
		message.Client.chatRoom.Broadcast(message.String())
	case strings.HasPrefix(message.Text, CMD_CREATE):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_CREATE + " "), "\n")
		fmt.Printf("Requested to create chat \"%s\"\n", name)
		err := lobby.AddChatRoom(NewChatRoom(name))
		if err != nil {
			message.Client.outgoing <- err.Error()
			return
		}
		serverMessage := NewMessage(time.Now(), serverClient, "Created chat room.")
		message.Client.outgoing <- serverMessage.String()
	case strings.HasPrefix(message.Text, CMD_LIST):
		fmt.Print("Requested to list chat rooms\n")
		lobby.ListChatRooms(message.Client)
	case strings.HasPrefix(message.Text, CMD_JOIN):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_JOIN + " "), "\n")
		err := lobby.JoinChatRoom(message.Client, name)
		if err != nil {
			message.Client.outgoing <- err.Error()
			return
		}
		serverMessage := NewMessage(time.Now(), serverClient, "Joined chat room.")
		message.Client.outgoing <- serverMessage.String()
	case strings.HasPrefix(message.Text, CMD_LEAVE):
		err := lobby.LeaveChatRoom(message.Client)
		if err != nil {
			message.Client.outgoing <- err.Error()
			return
		}
		serverMessage := NewMessage(time.Now(), serverClient, "Left chat room")
		message.Client.outgoing <- serverMessage.String()
	case strings.HasPrefix(message.Text, CMD_NAME):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_NAME + " "), "\n")
		// serverMessage := NewMessage(time.Now(), serverClient, fmt.Sprintf("\"%s\" changed their name to \"%s\".", message.Client.name, name))
		// lobby.Broadcast(serverMessage)
		message.Client.name = name
	case strings.HasPrefix(message.Text, CMD_HELP):
		fmt.Print("Requested help\n")
	}
}

func (lobby *Lobby) AddChatRoom(chatRoom *ChatRoom) error {
	if lobby.chatRooms[chatRoom.name] != nil {
		return errors.New(ERROR_CREATE)
	}
	lobby.chatRooms[chatRoom.name] = chatRoom
	go func() {
		time.Sleep(time.Now().AddDate(0, 0, 7).Sub(time.Now()))
		lobby.delete <- chatRoom
	}()
	return nil
}

func (lobby *Lobby) RemoveChatRoom(chatRoom *ChatRoom) {
	if chatRoom.expiry.After(time.Now()) {
		go func() {
			time.Sleep(chatRoom.expiry.Sub(time.Now()))
		}()
	} else {
		chatRoom.Delete()
		delete(lobby.chatRooms, chatRoom.name)
	}
}

func (lobby *Lobby) JoinChatRoom(client *Client, name string) error {
	if lobby.chatRooms[name] == nil {
		return errors.New(ERROR_JOIN)
	}
	if client.chatRoom != nil {
		err := lobby.LeaveChatRoom(client)
		if err != nil {
			return err
		}
	}
	lobby.chatRooms[name].Join(client)
	return nil
}

func (lobby *Lobby) LeaveChatRoom(client *Client) error {
	if client.chatRoom == nil {
		return errors.New(ERROR_LEAVE)
	}
	client.chatRoom.Leave(client)
	return nil
}

func (lobby *Lobby) ListChatRooms(client *Client) {
	client.outgoing <- fmt.Sprintf("Channels:\n")
	for name := range lobby.chatRooms {
		client.outgoing <- fmt.Sprintf("\"%s\"\n", name)
	}
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
		expiry:   time.Now().AddDate(0, 0, 7),
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
}

// Removes the given Client from the ChatRoom.
func (chatRoom *ChatRoom) Leave(client *Client) {
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
	chatRoom.expiry = time.Now()
	chatRoom.messages = append(chatRoom.messages, message)
	for _, client := range chatRoom.clients {
		client.outgoing <- message
	}
}

func (chatRoom *ChatRoom) Delete() {
	//notify of deletion?
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
	ln, err := net.Listen(CONN_TYPE, CONN_PORT)
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		os.Exit(1)
	}
		// Close the listener when the application closes.
	defer ln.Close()
	fmt.Println("Listening on " + CONN_PORT)
	for {
		// Listen for an incoming connection.
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			os.Exit(1)
		}
		// Handle connections in a new goroutine.
		go lobby.Join(NewClient(conn))
	}
}
