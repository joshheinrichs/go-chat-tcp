package main

import (
	"net"
	"bufio"
	"fmt"
	"os"
	"sync"
)

const (
    CONN_PORT = ":3333"
    CONN_TYPE = "tcp"
)

// Reads from the socket and outputs to the console.
func Read(conn net.Conn) {
	reader := bufio.NewReader(conn)
	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		fmt.Print(str)
	}
}

// Reads from Stdin, and outputs to the socket.
func Write(conn net.Conn) {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(conn)

	for {
		str, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		_, err = writer.WriteString(str)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		err = writer.Flush()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

// Starts up a read and write thread which connect to the server through the
// a socket connection.
func main() {
	var wg sync.WaitGroup

	conn, err := net.Dial(CONN_TYPE, CONN_PORT)
	if err != nil {
		fmt.Println(err)
	}

	wg.Add(2)

	go Read(conn)
	go Write(conn)

	wg.Wait()
}