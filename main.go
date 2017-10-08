package main

import (
	"github.com/nu7hatch/gouuid"
	_"github.com/mattn/go-sqlite3"
	tbot "gopkg.in/telegram-bot-api.v4"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
	pathutil "path"
	"strings"
	"strconv"
	"github.com/go-xorm/xorm"
)

var (
	bot *tbot.BotAPI
	db  *xorm.Engine
)

type Client struct {
	Chat int64
	Uuid string
}

func env(key, def string) (value string) {
	value = os.Getenv(key)
	if value != "" {
		return
	}
	return def
}

func envInt(key string, def int) (value int) {
	valueString := env(key, "")
	if valueString == "" {
		return def
	}
	var err error
	value64, err := strconv.ParseInt(valueString, 10, 32)
	value = int(value64)
	if err != nil {
		return def
	}
	return
}

func main() {

	var err error

	if env("ADDR_FAM", "unix") == "unix" {
		err = os.Remove(env("ADDR", "socket.sock"))
		if err != nil {
			log.Println("error removing socket:", err)
		} else {
			log.Println("removed socket")
		}
	}

	db, err = xorm.NewEngine("sqlite3", "./server.sqlite")
	if err != nil {
		log.Fatal(err)
	}

	err = db.Sync2(new(Client))
	if err != nil {
		log.Fatal(err)
	}

	go func() {
		http.HandleFunc("/", func(res http.ResponseWriter, req *http.Request) {
			path := strings.Split(pathutil.Clean(req.URL.Path), "/")

			if len(path) < 3 || path[0] != "" || path[1] != "notify" || len(path[2]) != 36 {
				http.NotFound(res, req)
				return
			}

			log.Println("notify request")

			message := req.URL.Query().Get("message")

			if message == "" {
				http.Error(res, "401 bad request - no message body", http.StatusBadRequest)
			}

			chat, err := getChat(path[2])
			if err != nil {
				log.Println(err)
				http.NotFound(res, req)
				return
			}

			msg := tbot.NewMessage(chat, message)
			_, err = bot.Send(msg)

			if err != nil {
				http.Error(res, "500 internal server error - message failed to send", http.StatusInternalServerError)
				return
			}

			res.Header().Set("Content-Type", "text/plain")
			res.Write([]byte("message sent"))
		})

		l, err := net.Listen(env("ADDR_FAM", "unix"), env("ADDR", "socket.sock"))
		if err != nil {
			log.Fatal(err)
		}

		if env("ADDR_FAM", "unix") == "unix" {
			err = os.Chmod(env("ADDR", "socket.sock"), 0777)
			if err != nil {
				log.Fatal("could not set mode of socket")
			}
		}

		log.Fatal(http.Serve(l, nil))

	}()

	{
		var err error
		tries := envInt("TRIES", 5)
		for try := 1; try <= tries; try++ {
			bot, err = tbot.NewBotAPI(env("API_KEY", ""))
			if err != nil {
				log.Println("could not authorise with telegram", err)
				if try == tries {
					log.Fatalf("giving up after %d tries", tries)
				}
				time.Sleep(time.Second * 60)
				continue
			}

			break
		}
	}

	bot.Debug = false

	log.Printf("authorized on account %s\n", bot.Self.UserName)

	u := tbot.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	log.Println("starting update loop")

	for update := range updates {
		if update.Message == nil {
			continue
		}
		if update.Message.Chat == nil {
			continue
		}
		if update.Message.Chat.Type != "private" {
			continue
		}

		if update.Message.IsCommand() != true {
			continue
		}

		command := update.Message.Command()

		if command == "register" {
			register(update.Message)
		}
	}
}

func register(message *tbot.Message) {
	id, err := uuid.NewV4()

	if err != nil {
		log.Println("error generating a new v4 uuid: ", err)
	}

	remove(message.Chat.ID)

	err = create(message.Chat.ID, id.String())
	if err != nil {
		log.Println("error saving new uuid: ", err)
	}

	msgString := `You will now recieve a message in this chat whenever this URL is requested:
` + env("BASEURL", "http://localhost/notify") + `/` + id.String() + `?message=your+message+here

You can replace the last part with your message

If you /register again you will recieve a new ID and invalidate the old one`

	msg := tbot.NewMessage(message.Chat.ID, msgString)
	bot.Send(msg)
}

func getChat(uuid string) (chat int64, err error) {
	var client Client
	var has bool

	has, err = db.Cols("chat").Where("uuid = ?", uuid).Get(&client)

	if ! has  {
		return 0, fmt.Errorf("no such client %s", uuid)
	}

	return client.Chat, err
}

func remove(chat int64) (err error) {
	client := &Client{Chat:chat}
	_, err = db.Delete(client)
	return
}

func create(chat int64, uuid string) (err error) {
	client := &Client{Chat:chat,Uuid:uuid}
	_, err = db.Insert(client)
	return
}
