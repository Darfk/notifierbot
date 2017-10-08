notifierbot
===

A scrap notifier bot for telegram, will send you a telegram message when a url is requested.

Tech Used:

- github.com/go-xorm/xorm
- github.com/mattn/go-sqlite3
- gopkg.in/telegram-bot-api.v4
- github.com/nu7hatch/gouuid

Install
---

    go get github.com/darfk/notifierbot
    go install github.com/darfk/notifierbot

Run
---

Provided your go executable path is in your PATH environment variable 

    $ API_KEY={your telegram bot key} \
    BASEURL={base url for webserver} \
    ADDR_FAM={web server bind address family, default=unix} \
    ADDR={web server bind address, default=socket.sock} \
    TRIES={number of times to try to connect to the Telegram API before giving up, default=5} \
    notifierbot

