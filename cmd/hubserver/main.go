package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func randStringBytes(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}
	return string(b)
}

type Subscriber struct {
	Callback     string
	LeaseSeconds int64
	Secret       string
}

type subscriptionHandler struct {
	Subscribers map[string][]Subscriber
}

func (handler *subscriptionHandler) handlePublish(w http.ResponseWriter, r *http.Request) error {
	topic := r.Form.Get("hub.topic")
	log.Printf("Topic = %s\n", topic)

	client := &http.Client{}
	res, err := client.Get(topic)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	feedContent, err := ioutil.ReadAll(res.Body)

	if subs, e := handler.Subscribers[topic]; e {
		for _, sub := range subs {
			req, err := http.NewRequest("POST", sub.Callback, strings.NewReader(string(feedContent)))
			if err != nil {
				log.Printf("While creating request to %s: %s", sub.Callback, err)
				continue
			}
			req.Header.Add("Content-Type", res.Header.Get("Content-Type"))
			req.Header.Add("Link",
				fmt.Sprintf(
					"<%s>; rel=hub, <%s>; rel=self",
					"https://hub.stuifzandapp.com/",
					topic,
				))
			if sub.Secret != "" {
				mac := hmac.New(sha1.New, []byte(sub.Secret))
				mac.Write(feedContent)
				signature := mac.Sum(nil)
				req.Header.Add("X-Hub-Signature", fmt.Sprintf("sha1=%x", signature))
			}
			res, err = client.Do(req)
			if err != nil {
				log.Printf("While POSTing to %s: %s", sub.Callback, err)
				continue
			}
		}
	}

	return nil
}

func (handler *subscriptionHandler) handleUnsubscription(w http.ResponseWriter, r *http.Request) error {
	log.Println(r.Form.Encode())
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	mode := r.Form.Get("hub.mode")

	if subs, e := handler.Subscribers[topic]; e {
		for i, sub := range subs {
			if sub.Callback != callback {
				continue
			}
			ourChallenge := randStringBytes(12)

			validationURL, err := url.Parse(callback)
			if err != nil {
				log.Println(err)
				return err
			}
			q := validationURL.Query()
			q.Add("hub.mode", mode)
			q.Add("hub.topic", topic)
			q.Add("hub.challenge", ourChallenge)
			validationURL.RawQuery = q.Encode()
			if validateURL(validationURL.String(), ourChallenge) {
				subs = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		w.WriteHeader(200)
		fmt.Fprintf(w, "Unsubscribed\n")
	} else {
		http.Error(w, "Hub does not handle subscription for topic", 400)
	}
	return nil
}

func (handler *subscriptionHandler) handleSubscription(w http.ResponseWriter, r *http.Request) error {
	log.Println(r.Form.Encode())
	callback := r.Form.Get("hub.callback")
	topic := r.Form.Get("hub.topic")
	leaseSecondsStr := r.Form.Get("hub.lease_seconds")
	leaseSeconds, err := strconv.ParseInt(leaseSecondsStr, 10, 64)
	if leaseSecondsStr != "" && err != nil {
		http.Error(w, "hub.lease_seconds is used, but not valid", 400)
		return nil
	}

	secret := r.Form.Get("hub.secret")
	callbackURL, err := url.Parse(callback)
	if err != nil {
		return err
	}

	topicURL, err := url.Parse(topic)
	if err != nil {
		return err
	}

	w.WriteHeader(202)
	fmt.Fprint(w, "Accepted")

	go func() {
		ourChallenge := randStringBytes(12)

		validationURL := *callbackURL
		q := validationURL.Query()
		q.Add("hub.mode", "subscribe")
		q.Add("hub.topic", topicURL.String())
		q.Add("hub.challenge", ourChallenge)
		q.Add("hub.lease_seconds", leaseSecondsStr)
		validationURL.RawQuery = q.Encode()

		log.Println(validationURL)

		if validateURL(validationURL.String(), ourChallenge) {
			handler.addSubscriberCallback(topicURL.String(), Subscriber{callbackURL.String(), leaseSeconds, secret})
		}
	}()

	return nil
}

func validateURL(url, challenge string) bool {
	client := http.Client{}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		log.Println(err)
		return false
	}
	res, err := client.Do(req)
	if err != nil {
		log.Println(err)
		return false
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Println(err)
		return false
	}

	return strings.Contains(string(body), challenge)
}

func (handler *subscriptionHandler) addSubscriberCallback(topic string, subscriber Subscriber) {
	defer handler.save()
	if subs, e := handler.Subscribers[topic]; e {
		for i, sub := range subs {
			if sub.Callback == subscriber.Callback {
				handler.Subscribers[topic][i] = subscriber
				return
			}
		}
	}

	// not found create a new subscription
	handler.Subscribers[topic] = append(handler.Subscribers[topic], subscriber)
}

func (handler *subscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		fmt.Println("WebSub hub")
		return
	}

	if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		http.Error(w, "Bad Request", 400)
		return
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}

	mode := r.Form.Get("hub.mode")

	if mode == "subscribe" {
		handler.handleSubscription(w, r)
		return
	} else if mode == "unsubscribe" {
		handler.handleUnsubscription(w, r)
		return
	} else if mode == "publish" {
		handler.handlePublish(w, r)
		return
	} else {
		http.Error(w, "Unknown hub.mode", 400)
		return
	}
}

func (handler *subscriptionHandler) load() error {
	file, err := os.Open("./subscription.json")
	if err != nil {
		if os.IsExist(err) {
			return err
		} else {
			handler.Subscribers = make(map[string][]Subscriber)
			return nil
		}
	}
	defer file.Close()
	dec := json.NewDecoder(file)
	dec.Decode(handler.Subscribers)
	return nil
}

func (handler *subscriptionHandler) save() error {
	file, err := os.Create("./subscription.json")
	if err != nil {
		return err
	}
	defer file.Close()
	dec := json.NewEncoder(file)
	dec.SetIndent("", "    ")
	dec.Encode(handler.Subscribers)
	return nil
}

func main() {
	handler := &subscriptionHandler{}
	handler.load()
	http.Handle("/", handler)
	log.Fatal(http.ListenAndServe(":80", nil))
}
