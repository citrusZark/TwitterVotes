package main

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/garyburd/go-oauth/oauth"
)

var (
	conn          net.Conn
	reader        io.ReadCloser
	authClient    *oauth.Client
	creds         *oauth.Credentials
	authSetupOnce sync.Once
	httpClient    *http.Client
	oauthConfig   twitterOauth
)

type tweet struct {
	Text string
}

type twitterOauth struct {
	ConsumerKey    string `json:"SP_TWITTER_KEY"`
	ConsumerSecret string `json:"SP_TWITTER_SECRET"`
	AccesToken     string `json:"SP_TWITTER_ACCESSTOKEN"`
	AccesSecret    string `json:"SP_TWITTER_ACCESSSECRET"`
}

func startTwitterStream(stopchan <-chan struct{}, votes chan<- string) <-chan struct{} {
	stoppedchan := make(chan struct{}, 1)
	go func() {
		defer func() {
			stoppedchan <- struct{}{}
		}()
		for {
			select {
			case <-stopchan:
				log.Println("stopping twitter....")
				return
			default:
				log.Println("querying twitter.")
				readFromTwitter(votes)
				log.Println("    (waiting) ")
				time.Sleep(10 * time.Second) //wait before reconnecting
			}
		}
	}()
	return stoppedchan
}

func readFromTwitter(votes chan<- string) {
	options, err := loadOptions()
	if err != nil {
		log.Println("failed to load option.")
		return
	}
	u, err := url.Parse("https://stream.twitter.com/1.1/statuses/filter.json")
	if err != nil {
		log.Println("creating filter request failed.")
		return
	}
	query := make(url.Values)
	query.Set("track", strings.Join(options, ","))
	req, err := http.NewRequest("POST", u.String(), strings.NewReader(query.Encode()))
	if err != nil {
		log.Println("creating filter request failed.")
		return
	}
	resp, err := makeRequest(req, query)
	if err != nil {
		log.Println("creating request failed.")
		return
	}
	reader := resp.Body
	decoder := json.NewDecoder(reader)
	for {
		var tweet tweet
		if err := decoder.Decode(&tweet); err != nil {
			break
		}
		log.Println("Got tweet: ", tweet.Text)
		for _, option := range options {
			if strings.Contains(
				strings.ToLower(tweet.Text),
				strings.ToLower(option),
			) {
				log.Println("vote:", option)
				votes <- option
			}
		}
	}
}

func makeRequest(req *http.Request, params url.Values) (*http.Response, error) {
	authSetupOnce.Do(func() {
		setupTwitterAuth()
		httpClient = &http.Client{
			Transport: &http.Transport{
				Dial: dial,
			},
		}
	})
	formEnc := params.Encode()
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Content-Length", strconv.Itoa(len(formEnc)))
	req.Header.Set("Authorization", authClient.AuthorizationHeader(creds, "POST", req.URL, params))
	return httpClient.Do(req)
}

func dial(netw, addr string) (net.Conn, error) {
	if conn != nil {
		conn.Close()
		conn = nil
	}
	netc, err := net.DialTimeout(netw, addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	conn = netc
	return netc, nil
}
func closeConn() {
	if conn != nil {
		conn.Close()
	}
	if reader != nil {
		reader.Close()
	}
}
func setupOauthConfig() {
	file, err := os.Open("oauth.json")
	defer file.Close()
	if err != nil {
		log.Fatalf("[loadOauth]: %s\n", err)
	}
	decoder := json.NewDecoder(file)
	oauthConfig = twitterOauth{}
	err = decoder.Decode(&oauthConfig)
	if err != nil {
		log.Fatalf("[loadOauthConfig]: %s\n", err)
	}
}
func setupTwitterAuth() {
	creds = &oauth.Credentials{
		Token:  oauthConfig.AccesToken,
		Secret: oauthConfig.AccesSecret,
	}
	authClient = &oauth.Client{
		Credentials: oauth.Credentials{
			Token:  oauthConfig.ConsumerKey,
			Secret: oauthConfig.ConsumerSecret,
		},
	}
}
