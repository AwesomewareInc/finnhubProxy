package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
)

var LocalConfig struct {
	FinnhubKey 		string
	WatchStocks 	[]string
	Interval		string	
}

type CachedStocksType struct {
	sync.Mutex
	inner map[string]*StockInfo
}
var cachedStocks CachedStocksType

type StockInfo struct {
	CurrentPrice 		float32		`json:"c"`
	Change 				float32		`json:"d"`
	ChangePercent 		float32		`json:"dp"`
	HighPriceOfTheDay	float32		`json:"h"`
	LowPriceOfTheDay	float32		`json:"l"`
	OpenPriceOfTheDay	float32		`json:"o"`
	PreviousClosePrice	float32		`json:"pc"`
}

func getStock(symbol string) (*StockInfo, error) {
	resp, err := http.Get("https://finnhub.io/api/v1/quote?symbol="+symbol+"&token="+LocalConfig.FinnhubKey+"");
	if err != nil {
		return nil, err;
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err;
	}

	var inf StockInfo
	if err := json.Unmarshal(body, &inf); err != nil {
		return nil, err;
	}
	return &inf, nil
}

func cacheAll() {
	var c CachedStocksType
	c.inner = make(map[string]*StockInfo)
	for _, symbol := range LocalConfig.WatchStocks {
		if stock, err := getStock(symbol); err != nil {
			fmt.Println(err.Error())
		} else {
			c.inner[symbol] = stock;
		}
		fmt.Println("Caching "+symbol);
		time.Sleep((1 * time.Second) + (500 * time.Millisecond))
	}
}

func main() {
	var duration time.Duration
	var err error
	if LocalConfig.Interval != "" {
		duration, err = time.ParseDuration(LocalConfig.Interval)
		if err != nil {
			fmt.Println(err)
			return
		}
	} else {
		duration = time.Duration(time.Minute * 30)
	}
	ticker := time.NewTicker (duration)

	cachedStocks.inner = make(map[string]*StockInfo)
	// set up the config
	f, err := os.Open("config.toml")
	if err != nil {
		fmt.Println(err)
		return
	} else {
		_, err = toml.NewDecoder(f).Decode(&LocalConfig)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	done := make(chan bool)

	// Start caching the stuff.
	go func() {
		// we want to cache every thing when the program starts up and THEN every 30 minutes.
		cacheAll() 
		for {
			select {
				case <- done:
					return
				case _ = <-ticker.C:
					cacheAll()
			}
		}
	}();

	s := &http.Server{
		Addr:	":8969",
		Handler: http.HandlerFunc(root),
		ReadTimeout: 10 * time.Second,
		WriteTimeout: 10 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())
}

func root(w http.ResponseWriter, r *http.Request) {
	path := strings.ReplaceAll(r.URL.Path, "/","");
	if(path == "") {
		w.Write([]byte(":3"))
	} else {
		cachedStocks.Lock()
		defer cachedStocks.Unlock()
		r.Header.Add("Content-Type", "application/json")

		if cachedStocks.inner[path] != nil {
			
			if bytes, err := json.Marshal(cachedStocks.inner[path]); err != nil {
				w.Write([]byte("{\"error\": \""+err.Error()+"\"}"))
			} else {
				w.Write(bytes)
			}
		} else {
			var found *string;
			for _, st := range LocalConfig.WatchStocks {
				if(st == path) {
					found = &path;
					break;
				}
			}
			if found == nil {
				w.Write([]byte("{\"error\": \"Not a watched stock.\"}"))
			} else {
				if stock, err := getStock(path); err != nil {
					w.Write([]byte("{\"error\": \""+err.Error()+"\"}"))
				} else {
					cachedStocks.inner[path] = stock;
					if bytes, err := json.Marshal(stock); err != nil {
						w.Write([]byte("{\"error\": \""+err.Error()+"\"}"))
					} else {
						w.Write(bytes)
					}
				}
			}
		}
	}
}