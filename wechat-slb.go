package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Servers []string `json:"servers"`
	Routes  []Route  `json:"routes"`
	Port    string   `json:"port`
}

type Route struct {
	Route     string   `json:"route"`
	Endpoints []string `json:"endpoints"`
}

func Parse(configFile string) Config {
	var config = Config{}
	data, err := ioutil.ReadFile(configFile)
	err = json.Unmarshal(data, &config)
	if err != nil {
		panic(err)
	}
	if len(config.Servers) == 0 {
		config.Servers = []string{"http://token.zhujq.ga"}
	}
	return config
}

var config = Config{}
var count map[int]int

//Server key is -1
const serverMethod = -1

func proxy(target string, w http.ResponseWriter, r *http.Request) {
	url, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(url)

	r.URL.Host = url.Host
	r.URL.Scheme = url.Scheme
	//	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = url.Host

	proxy.ServeHTTP(w, r)
}

//HTTPGet get 请求，用于健康检查
func HTTPGet(uri string) bool {
	response, err := http.Get(uri)
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}

	return true
}

func handle(w http.ResponseWriter, r *http.Request) {
	baseURL := r.URL.Path[1:]
	baseURL = strings.Split(baseURL, "/")[0]
	writeToLog("Basepath: /" + baseURL)
	if len(config.Servers) > 0 {

		server := chooseServer(config.Servers, serverMethod)
		writeToLog("Healthy Server: " + server)
		proxy(server, w, r)
		/*
			for {
				server := chooseServer(config.Servers, serverMethod)
				if HTTPGet(server) == true {
					writeToLog("Healthy Server: " + server)
					proxy(server, w, r)
					break
				}

			}
		*/
	} else if len(config.Routes) > 0 {
		for m := range config.Routes {
			route := config.Routes[m].Route
			bURL := strings.Split(route, "/")[1]
			if baseURL == bURL {
				server := chooseServer(config.Routes[m].Endpoints, m)
				writeToLog("Route: " + server)
				proxy(server, w, r)
			}
		}
	}
}

func chooseServer(servers []string, method int) string {
	for {
		count[method] = (count[method] + 1) % len(servers)
		if servers[count[method]] != "" {
			writeToLog("Chose server: " + servers[count[method]])
			return servers[count[method]]
		}
	}
}

func writeToLog(message string) {
	logFile, err := os.OpenFile("log.txt", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		panic(err)
	}
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Println(message)
	logFile.Close()
}

//Could be improved but gets the job done
func reloadConfig(configFile string, config chan Config, wg *sync.WaitGroup) {

	var oldConfig Config
	var t Config
	for {
		t = Parse(configFile)
		for i, wcserver := range t.Servers {
			if HTTPGet(wcserver) == false {
				t.Servers[i] = "" //不可达服务器置为空
				writeToLog(wcserver + "is not alive!")
			}
		}
		//	fmt.Println(reflect.DeepEqual(t, oldConfig))
		if !reflect.DeepEqual(t, oldConfig) {
			config <- t
			writeToLog("Reloaded config")
			oldConfig = t
		}

		time.Sleep(600 * time.Second) //每10分钟刷新一次配置
	}
	close(config)
	wg.Done()
	return
}

func launch(server *http.Server, wg *sync.WaitGroup) {
	writeToLog("Port: " + server.Addr)
	writeToLog("Starting http slb service...")
	handler := http.HandlerFunc(handle)
	server.Handler = handler
	server.ListenAndServe()
	wg.Done()
}

func main() {
	var configFile = "./slb.json"
	var server *http.Server
	var wg sync.WaitGroup

	// Adding the reload and exit goroutines
	wg.Add(2)

	count = make(map[int]int)

	configChannel := make(chan Config)

	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	go reloadConfig(configFile, configChannel, &wg)

	go func() {
		for config = range configChannel {

			port := ":" + config.Port
			if port == ":" {
				port = port + "8080"
			}
			//		fmt.Println(server)
			if server != nil {
				writeToLog("Server closing: " + server.Addr)
				//	fmt.Println("Server closing...")
				server.Close()
			}
			server = &http.Server{
				Addr:         port,
				ReadTimeout:  5 * time.Second,
				WriteTimeout: 10 * time.Second,
			}
			wg.Add(1)
			go launch(server, &wg)
		}
		writeToLog("The Web Service is Exited")
		wg.Done()
	}()

	wg.Wait()
}
