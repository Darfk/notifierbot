package main

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/nu7hatch/gouuid"
	tbot "gopkg.in/telegram-bot-api.v4"
	_ "io/ioutil"
	"log"
	_ "math/rand"
	"net"
	"net/http"
	"os"
	pathutil "path"
	"strings"
)

var (
	bot *tbot.BotAPI
	db  *bolt.DB
)

func env(key, def string) (value string) {
	value = os.Getenv(key)
	if value != "" {
		return
	}
	return def
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

	db, err = bolt.Open("server.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	log.Println("started database")

	err = db.Update(func(tx *bolt.Tx) error {
		var err error

		_, err = tx.CreateBucketIfNotExists([]byte("chats"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		_, err = tx.CreateBucketIfNotExists([]byte("uuids"))
		if err != nil {
			return fmt.Errorf("create bucket: %s", err)
		}
		return nil
	})

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
				http.NotFound(res, req)
				return
			}

			msg := tbot.NewMessage(chat, message)
			_, err = bot.Send(msg)

			if err != nil {
				http.Error(res, "500 internal server error - message failed to send", http.StatusInternalServerError)
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

	bot, err = tbot.NewBotAPI(env("API_KEY", ""))
	if err != nil {
		log.Println("could not authorise with telegram", err)
		return
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
	var chatBuf *bytes.Buffer
	var uuidBuf *bytes.Buffer = &bytes.Buffer{}

	err = gob.NewEncoder(uuidBuf).Encode(uuid)
	if err != nil {
		return
	}

	err = db.View(func(tx *bolt.Tx) error {
		var buk *bolt.Bucket
		buk = tx.Bucket([]byte("chats"))
		chatBuf = bytes.NewBuffer(buk.Get(uuidBuf.Bytes()))
		return nil
	})
	if err != nil {
		return
	}

	err = gob.NewDecoder(chatBuf).Decode(&chat)
	if err != nil {
		return
	}

	return
}

func remove(chat int64) (err error) {
	var chatBuf *bytes.Buffer = &bytes.Buffer{}
	var uuidBuf *bytes.Buffer

	err = gob.NewEncoder(chatBuf).Encode(chat)
	if err != nil {
		return err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		var uuidBuk *bolt.Bucket
		var chatBuk *bolt.Bucket

		chatBuk = tx.Bucket([]byte("chats"))
		uuidBuk = tx.Bucket([]byte("uuids"))
		uuidBuf = bytes.NewBuffer(uuidBuk.Get(chatBuf.Bytes()))

		err = chatBuk.Delete(uuidBuf.Bytes())
		if err != nil {
			log.Println(err)
		}

		err = uuidBuk.Delete(chatBuf.Bytes())
		if err != nil {
			log.Println(err)
		}

		return nil

	})

	return nil
}

func create(chat int64, uuid string) error {
	var err error

	var chatBuf bytes.Buffer
	err = gob.NewEncoder(&chatBuf).Encode(chat)
	if err != nil {
		return err
	}

	var uuidBuf bytes.Buffer
	err = gob.NewEncoder(&uuidBuf).Encode(uuid)
	if err != nil {
		return err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		var err error
		var buk *bolt.Bucket

		buk = tx.Bucket([]byte("uuids"))
		err = buk.Put(chatBuf.Bytes(), uuidBuf.Bytes())
		if err != nil {
			return err
		}

		buk = tx.Bucket([]byte("chats"))
		err = buk.Put(uuidBuf.Bytes(), chatBuf.Bytes())
		if err != nil {
			return err
		}

		return nil
	})
	return nil
}
