// main
package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	var addr = flag.String("addr", ":8082", "website address.")
	flag.Parse()
	mux := http.NewServeMux()
	mux.Handle("/", http.StripPrefix("/", http.FileServer(http.Dir("src"))))
	log.Println("Serving web at: ", *addr)
	http.ListenAndServe(*addr, mux)
}
