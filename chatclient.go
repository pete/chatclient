package main

import (
	"fmt"
	"log"
	"os"
	"time"
	"bufio"
	"net/http"
	"encoding/json"

	"golang.org/x/net/websocket"
)

type PleromaSection struct {
	ChatToken string `json:"chat_token"`
}
type AuthResp struct {
	Pleroma *PleromaSection `json:"pleroma"`
}
type MsgTxt struct {
	Text string `json:"text"`
}

var degug bool
var m *log.Logger

func main() {
	degug = os.Getenv("degug") != ""
	// Determine hostname, username, password
	user := os.Getenv("user")
	pass := os.Getenv("pass")
	if pass == "" || user == "" || len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s $hostname\n" +
			"$ user/pass are extracted from environment variables.\n"+
			"Brave people can pass them inline like:\n" +
			"$ user=p pass=noodles %s freespeechextremist.com\n" +
			"(But you might want to set them in a file that is chmod 600'd and\n"+
			"source that in a subshell if you wanna be careful.)\n" +
			"Factotum support left as an exercise for the reader.\n",
			os.Args[0], os.Args[0])
		os.Exit(1)
	}
	host := os.Args[1]
	
	// Hit verify_credentials
	// Extract pleroma.chat_token, set that.
	ct, err := chattok(host, user, pass)
	if err != nil {
		log.Fatal(err) 
	}

	// Connect to Websocket
	origin := fmt.Sprintf("https://%s/", host)
	url := fmt.Sprintf("wss://%s/socket/websocket?token=%s&vsn=2.0.0", host, ct)
	ws, err := websocket.Dial(url, "", origin)
	if err != nil {
		log.Fatal(err)
	}

	// Join the chat:  ["1","1","chat:public","phx_join",{}]
	_, err = ws.Write([]byte(`["1","1","chat:public","phx_join",{}]`))
	if err != nil {
		log.Fatal(err)
	}

	c := idmfg(1)
	hmsg := `[null,"%d","phoenix","heartbeat",{}]`
	go func() {
		for {
			time.Sleep(30 * time.Second)
			_, err := ws.Write([]byte(fmt.Sprintf(hmsg, <-c)))
			if err != nil {
				log.Fatal(err)
			}
		}
	}()

	m = log.New(os.Stdout, log.Prefix(), log.Flags())

	go inputloop(ws, c)

	dc := json.NewDecoder(ws)

	// Start the loop
	for {
		var f []interface{}
		err := dc.Decode(&f)

		if err != nil {
			log.Fatal(err)
		}
		if len(f) != 5 {
			log.Fatalf(
				"Expected five elements in the array, got %d in %#v",
				len(f), f)
		}
		typ, ok := f[3].(string)
		if !ok {
			log.Fatalf("Message type isn't a string, can't cope. (%#v)",
				f)
		}

		if degug {
			log.Printf("[%s] %#v\n", typ, f)
		}

		// If it's busted, let's just crash.
		switch typ {
		case "phx_reply":
			// No action required:  we get this for heartbeats and
			// successful messages, but there's no action required.
			// We would spit it out for debugging, but all messages
			// are currently spat out above.
		case "messages":
			msgs := f[4].(map[string]interface{})["messages"]
			printmsgs(user, msgs.([]interface{}))
		case "new_msg":
			printmsg(user, f[4].(map[string]interface{}))
		default:
			log.Printf("[%s] IS UNHANDLED %#v", typ, f[4])
		}
	}
}

func inputloop(ws *websocket.Conn, c chan int) {
	var a [5]interface{}
	e := json.NewEncoder(ws)
	r := bufio.NewReader(os.Stdin)
	for {
		l, err := r.ReadString('\n')
		if err != nil {
			log.Fatalf("input: %v", err)
		}
		if len(l) < 2 {
			continue
		}

		a = [5]interface{}{
			"1",
			fmt.Sprintf("%d", <-c),
			"chat:public",
			"new_msg",
			&MsgTxt{Text: l[0:len(l)-1]},
		}
		err = e.Encode(a)
		if err != nil {
			log.Fatalf("sending input: %v", err)
		}
	}
}

func printmsgs(user string, msgs []interface{}) {
	for _, v := range(msgs) {
		printmsg(user, v.(map[string]interface{}))
	}
}

func printmsg(user string, msg map[string]interface{}) {
	// Try to grab author name; it's not present when the message
	// is from the user.  Walking down a JSON tree is a pain in Go.
	// msg['author']['acct'] ⇐ This should be short.
	author, ok := msg["author"].(map[string]interface{})
	if ok {
		acct, ok := author["acct"].(string)
		if ok {
			user = acct
		}
	}
	text, ok := msg["text"].(string)
	if !ok {
		log.Printf("Misunderstood this message:  %#v", msg)
		return
	}
	m.Printf("[%s]:  %s", user, text)
}

func idmfg(i int) chan int {
	c := make(chan int)
	go func() {
		for {
			i++
			c <- i
		}
	}()
	return c
}

func chattok(h, u, p string) (string, error) {
	c := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return fmt.Errorf("redirect")
		},
	}
	url := fmt.Sprintf("https://%s/api/v1/accounts/verify_credentials", h)
	req, err := http.NewRequest("GET", url, nil)
	req.SetBasicAuth(u, p)
	resp, err := c.Do(req)
	if resp != nil && resp.StatusCode != 200 {
		log.Printf("Server returned %d\n\t(%v)\n", resp.StatusCode, resp)
		return "", fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	if err != nil {
		return "", err
	}

	a := &AuthResp{}
	err = json.NewDecoder(resp.Body).Decode(&a)
	if err != nil {
		return "", err
	}

	return a.Pleroma.ChatToken, nil
}
