package main

import (
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const webPort = "8080"

const (
	dev_user        = "guest"
	dev_pwd         = "guest"
	maxConnectCount = 5
)

type Config struct {
	Rabbit *amqp.Connection
}

func main() {
	// Connect to RabbitMQ (DB)
	rabbitConn, err := connectRabbitMq()
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
	defer rabbitConn.Close()

	app := Config{
		Rabbit: rabbitConn,
	}

	log.Printf("(TEST) Start to run broker service at port %s\n", webPort)

	// define http server
	server := &http.Server{
		Addr:    fmt.Sprintf(":%s", webPort),
		Handler: app.Router(),
	}

	// run server
	err = server.ListenAndServe()
	if err != nil {
		log.Panic(err)
	}
}

func connectRabbitMq() (*amqp.Connection, error) {
	var counts int64
	var connection *amqp.Connection
	var backOff = 1 * time.Second

	for {
		c, err := amqp.Dial("amqp://" + dev_user + ":" + dev_pwd + "@rabbitmq")
		if err != nil {
			fmt.Println("RabbitMQ not ready...")
			counts++
		} else {
			connection = c
			fmt.Println("Connected to RabbitMQ!")
			break // break the for loop
		}

		if counts > maxConnectCount {
			fmt.Println(err)
			return nil, err
		}

		backOff = time.Duration(math.Pow(float64(counts), 2)) * time.Second
		log.Println("backing off...")
		time.Sleep(backOff)
		continue
	}

	return connection, nil
}
