// main
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	nsq "github.com/bitly/go-nsq"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

const updateDuration = 5 * time.Second

var (
	fatalErr   error
	counts     map[string]int
	countsLock sync.Mutex
)

func fatal(e error) {
	fmt.Println(e)
	flag.PrintDefaults()
	fatalErr = e
}

func main() {
	defer func() {
		if fatalErr != nil {
			os.Exit(1)
		}
	}()

	//connect db
	log.Println("Connecting to database...")
	db, err := mgo.Dial("localhost")
	if err != nil {
		fatal(err)
		return
	}
	defer func() {
		log.Println("Closing database connection...")
		db.Close()
	}()
	pollData := db.DB("ballots").C("polls")

	//connect nsq
	log.Println("Connecting to nsq...")
	q, err := nsq.NewConsumer("votes", "counter", nsq.NewConfig())
	if err != nil {
		fatal(err)
		return
	}
	q.AddHandler(nsq.HandlerFunc(func(m *nsq.Message) error {
		countsLock.Lock()
		defer countsLock.Unlock()
		if counts == nil {
			counts = make(map[string]int)
		}
		vote := string(m.Body)
		counts[vote]++
		log.Println("getting vote from nsq: ", counts)
		return nil
	}))
	if err := q.ConnectToNSQLookupd("localhost:4161"); err != nil {
		fatal(err)
		return
	}

	//update db
	log.Println("Waiting for votes on nsq...")
	//	var updater *time.Timer
	//	updater = time.AfterFunc(updateDuration, func() {
	//		countsLock.Lock()
	//		defer countsLock.Unlock()
	//		if len(counts) == 0 {
	//			log.Println("No new votes, skipping database update.")
	//		} else {
	//			log.Println("Updating database....")
	//			log.Println(counts)
	//			ok := true
	//			for option, count := range counts {
	//				sel := bson.M{"options": bson.M{"$in": []string{option}}}
	//				up := bson.M{"$inc": bson.M{"results." + option: count}}
	//				if _, err := pollData.UpdateAll(sel, up); err != nil {
	//					log.Println("failed to update: ", err)
	//					ok = false
	//				}
	//			}
	//			if ok {
	//				log.Println("Finished updating database...")
	//				counts = nil //reset counts
	//			}
	//		}
	//		updater.Reset(updateDuration)
	//	})
	var updaterStopCh = make(chan struct{}, 1)
	go func() {
		var ticker = time.NewTicker(updateDuration)
		for {
			select {
			case <-ticker.C:
			case <-updaterStopCh:
				log.Println("Stopping updater...")
				ticker.Stop()
				return
			}
			func() {
				countsLock.Lock()
				defer countsLock.Unlock()
				if len(counts) == 0 {
					log.Println("No new votes, skipping database update.")
				} else {
					log.Println("Updating database....")
					log.Println(counts)
					ok := true
					for option, count := range counts {
						sel := bson.M{"options": bson.M{"$in": []string{option}}}
						up := bson.M{"$inc": bson.M{"results." + option: count}}
						if _, err := pollData.UpdateAll(sel, up); err != nil {
							log.Println("failed to update: ", err)
							ok = false
						}
					}
					if ok {
						log.Println("Finished updating database...")
						counts = nil //reset counts
						log.Println("Clearing counter: ", counts)
					}
				}
			}()
		}
	}()
	termChan := make(chan os.Signal, 1)
	signal.Notify(termChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		select {
		case <-termChan:
			//updater.Stop()
			close(updaterStopCh) //this get same result as updaterStopCh <- struct{}{}
			q.Stop()             //this actually, q send signal to q.stopChan, so in case below if <-q.stoChan then program exit
		case <-q.StopChan:
			//finished
			return
		}
	}
}
