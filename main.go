package main

import (
	"log"
	"net/http"

	"github.com/dingtalk-alert-server/server"
)

func main() {
	log.Print("[INFO] Dingtalk server start")
	http.HandleFunc("/dingtalk", server.ReceiveAndSend)

	err := http.ListenAndServe(":9090", nil)
	if err != nil {
		log.Fatal(err)
	}
}
