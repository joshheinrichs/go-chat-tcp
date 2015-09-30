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

	CMD_CREATE = "/create"
	CMD_LIST = "/list"
	CMD_JOIN = "/join"
	CMD_LEAVE = "/leave"
	CMD_HELP = "/help"
	CMD_NAME = "/name"

	DEFAULT_NAME = "Anonymous"
)

var chatRooms map[string]*ChatRoom
var lobby *Lobby
var serverClient *Client = &Client {
	Name: "Server",
	incoming: nil,
	outgoing: nil,
	reader: nil,
	writer: nil,
}

type Lobby struct {
	clients  []*Client
	messages []*Message
	incoming chan *Message
	end      map[*Client]chan bool
	join     chan *Client
	leave    chan *Client
}

func NewLobby() *Lobby {
	lobby := &Lobby {
		clients:  make([]*Client, 0),
		messages: make([]*Message, 0),
		incoming: make(chan *Message),
		join:     make(chan *Client),
	}
	lobby.Listen()
	return lobby
}

//waits for any data to be passed over the channels
func (lobby *Lobby) Listen() {
	go func() {
		for {
			select {
			case message := <-lobby.incoming:
				lobby.Parse(message)
			case client := <-lobby.join:
				lobby.Join(client)
			}
		}
	}()
}

//join the lobby
func (lobby *Lobby) Join(client *Client) {
	lobby.clients = append(lobby.clients, client)

	for _, message := range lobby.messages {
		client.outgoing <- message
	}

	go func() {
		for { 
			lobby.incoming <- <-client.incoming
		} 
	}()
}

func (lobby *Lobby) Leave(client *Client) {

}

func (lobby *Lobby) Broadcast(message *Message) {
	for _, client := range lobby.clients {
		client.outgoing <- message
	}
}

func (lobby *Lobby) Parse(message *Message) {
	switch {
	default:
		fmt.Printf("%s", message)
		lobby.Broadcast(message)
		lobby.messages = append(lobby.messages, message)
	case strings.HasPrefix(message.Text, CMD_CREATE):
		chat := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_CREATE + " "), "\n")
		fmt.Printf("Requested to create chat \"%s\"\n", chat) 
	case strings.HasPrefix(message.Text, CMD_LIST):
		fmt.Print("Requested to list channels\n")
	case strings.HasPrefix(message.Text, CMD_JOIN): 
		chat := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_JOIN + " "), "\n")
		fmt.Printf("Requested to join chat \"%s\"\n", chat)
	case strings.HasPrefix(message.Text, CMD_LEAVE):
		serverMessage := NewMessage(time.Now(), serverClient, "You cannot leave the lobby.")
		message.Client.outgoing <- serverMessage
		fmt.Printf("Requested to leave channel\n")
	case strings.HasPrefix(message.Text, CMD_NAME):
		name := strings.TrimSuffix(strings.TrimPrefix(message.Text, CMD_NAME + " "), "\n")
		serverMessage := NewMessage(time.Now(), serverClient, fmt.Sprintf("\"%s\" changed their name to \"%s\".", message.Client.Name, name))
		lobby.Broadcast(serverMessage)
		message.Client.Name = name
	case strings.HasPrefix(message.Text, CMD_HELP):
		fmt.Print("Requested help\n")
	}
}

type ChatRoom struct {
	clients  []*Client
	incoming chan *Message
	join     chan *Client
	leave    chan *Client
	delete   chan bool
	expiry   time.Time
}

func (chatRoom *ChatRoom) Join(client *Client) {
}

func (chatRoom *ChatRoom) Leave(client *Client) {
}

func (chatRoom *ChatRoom) Delete() {

}

func (chatRoom *ChatRoom) Parse(message *Message) {
	str := message.Text
	switch {
	default:
		fmt.Print(str)
	case strings.HasPrefix(str, CMD_CREATE):
		chat := strings.TrimSuffix(strings.TrimPrefix(str, CMD_CREATE + " "), "\n")
		fmt.Printf("Requested to create chat \"%s\"\n", chat) 
	case strings.HasPrefix(str, CMD_LIST):
		fmt.Print("Requested to list channels\n") 
	case strings.HasPrefix(str, CMD_JOIN): 
		chat := strings.TrimSuffix(strings.TrimPrefix(str, CMD_JOIN + " "), "\n")
		fmt.Printf("Requested to join chat \"%s\"\n", chat)
	case strings.HasPrefix(str, CMD_LEAVE):
		fmt.Printf("Requested to leave channel\n")
	case strings.HasPrefix(str, CMD_NAME):
		name := strings.TrimSuffix(strings.TrimPrefix(str, CMD_NAME + " "), "\n")
		fmt.Printf("Requested to change name to \"%s\"\n", name)
		message.Client.Name = name
	case strings.HasPrefix(str, CMD_HELP):
		fmt.Print("Requested help\n")
	}
}

func NewChatRoom(client *Client, name string) {

}

type Client struct {
	Name     string
	incoming chan *Message
	outgoing chan *Message
	reader   *bufio.Reader
	writer   *bufio.Writer
}

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

func (client *Client) Write() {
	for message := range client.outgoing {
		client.writer.WriteString(message.String())
		client.writer.Flush()
	}
}

func NewClient(conn net.Conn) *Client {
	writer := bufio.NewWriter(conn)
	reader := bufio.NewReader(conn)

	client := &Client {
		Name: "Anonymous",
		incoming: make(chan *Message),
		outgoing: make(chan *Message),
		reader: reader,
		writer: writer,
	}

	go client.Read()
	go client.Write()

	return client
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
	return message;
}

func (message *Message) String() string {
	return fmt.Sprintf("%s - %s: %s\n", message.Time.Format(time.Kitchen), message.Client.Name, message.Text)
}

func main() {
	chatRooms = make(map[string]*ChatRoom)
	lobby = NewLobby()

	// Listen for incoming connections.
	ln, err := net.Listen(CONN_TYPE, CONN_PORT)
	if err != nil {
		fmt.Println("Error listening:", err.Error())
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
