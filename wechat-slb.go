package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Servers []string `json:"servers"`
	Delay   []int    `json:"delay"`
	Routes  []Route  `json:"routes"`
	Port    string   `json:"port"`
	Mode    string   `json:"mode"`
	Theone  int      `json:"theone"`
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
		config.Servers = []string{"http://wechat.zhujq.ga"}
	}
	return config
}

// Server key is -1
const serverMethod = -1

var config = Config{}
var count map[int]int

func proxy(target string, w http.ResponseWriter, r *http.Request) {
	url, _ := url.Parse(target)
	proxy := httputil.NewSingleHostReverseProxy(url)

	r.URL.Host = url.Host
	r.URL.Scheme = url.Scheme
	//	r.Header.Set("X-Forwarded-Host", r.Header.Get("Host"))
	r.Host = url.Host

	proxy.ServeHTTP(w, r)
}

// HTTPGet get 请求，用于健康检查
func HTTPGet(uri string) bool {
	response, err := http.Get(uri + "/healthck")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}

	b, _ := ioutil.ReadAll(response.Body)
	if string(b) != "ok" {
		return false
	}
	return true
}

func writeconf() {
	file, _ := os.OpenFile("./slb.json", os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0666)
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	encoder.Encode(config)
}

func handle(w http.ResponseWriter, r *http.Request) {
	baseURL := r.URL.Path[1:]
	baseURL = strings.Split(baseURL, "/")[0]
	writeToLog("Basepath: / " + baseURL)
	if baseURL == "manager" {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		result := `<html><head><title>SLB Server Status</title><meta http-equiv="pragma" content="no-cache"><meta http-equiv="cache-control" content="no-cache"><meta http-equiv="expires" content="0"><meta http-equiv="Content-Type" content="text/html; charset=utf-8"></head>`
		result += `<style>body{ font-family:"微软雅黑"}</style>`
		result += `<body style="text-align:center;background-color:Turquoise"><div style="border:1px solid #F00;width:600px;height:450px ;margin:auto;text-align:left;position:fixed;top:180;left:0;right:0">SLB Server is running on port: `
		result += `<b><font color="red">` + config.Port + `</font></b>，SLB Mode is：<b><font color="red">`
		result += config.Mode + "</font></b>"
		result += `<form action="chgmode">Random<input type="radio" name="mode" value="random">&nbsp;&nbsp;Best<input type="radio" name="mode" value="best">&nbsp;&nbsp;single<input type="radio" name="mode" value="single">&nbsp;&nbsp;&nbsp;&nbsp;<input type="submit" value="Mode-Switch" style="color:Red ;background-color:Turquoise;width:150px;font-weight:bold"></form>`
		if config.Mode == "single" {
			result += `<form action="choosesingle" method="get"> `
		} else {
			result += `<form action="delslbserver" method="get"> `
		}
		result += `<hr>SLB Backend Server's Delay(ms):<table border=2><tr><td>No.</td><td>Backend URL</td><td>Delay</td>`
		if config.Mode == "single" {
			result += "<td>To Choose Single Backend</td>"
		} else {
			result += "<td>To DEL</td>"
		}
		for index, val := range config.Servers {
			result += "<tr><td>"
			result += strconv.Itoa(index)
			result += "</td><td>"
			result += val
			result += "</td><td>"
			result += strconv.Itoa(config.Delay[index])
			if config.Mode == "single" && config.Theone == index {
				result += `</td><td><input type="radio" name="delslbindex" checked="checked" value="`
			} else {
				result += `</td><td><input type="radio" name="delslbindex" value="`
			}
			result += strconv.Itoa(index)
			result += `">`
			result += "</td></tr>"
		}
		if config.Mode == "single" {
			result += `<tr><td colspan="4"  align="right"><input type="submit" value="Choose"> </td></table></form>`
		} else {
			result += `<tr><td colspan="4"  align="right"><input type="submit" value="Del"> </td></table></form>`
		}
		result += `<hr><form action="addslbserver" method="get">New SLB Backend URL:<br><input type="text" name="newslbserver" style="width:300px"> <input type="submit" value="Add"></form></div>`
		result += "</body></html>"
		fmt.Fprintf(w, result)

		return
	}

	if baseURL == "chgmode" {

		m, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(m) == 0 || strings.Contains(r.URL.RawQuery, "mode") == false {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "choose mode error")
			return
		}

		mode := m["mode"][0]
		if len(mode) == 0 {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "choose mode error")
			return
		}

		if mode != "random" && mode != "best" && mode != "single" {
			mode = "random"
		}
		config.Mode = mode

		writeconf()
		http.Redirect(w, r, "/manager", http.StatusTemporaryRedirect)
		return
	}

	if baseURL == "addslbserver" {
		m, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(m) == 0 || strings.Contains(r.URL.RawQuery, "newslbserver") == false {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "add slb backend url error")
			return
		}
		newslbs := m["newslbserver"][0]
		if len(newslbs) == 0 {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "add slb backend url error")
			return
		}
		_, err = url.ParseRequestURI(newslbs)
		if err != nil {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, (newslbs + " is a wrong url"))
			return
		}
		//	log.Println(newslbs)
		config.Servers = append(config.Servers, newslbs)
		config.Delay = append(config.Delay, 0)

		writeconf()

		http.Redirect(w, r, "/manager", http.StatusTemporaryRedirect)

		return
	}

	if baseURL == "delslbserver" {
		m, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(m) == 0 {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "delete slb backend url error")
			return
		}
		delslbindex, err := strconv.Atoi(m["delslbindex"][0])

		if err != nil || delslbindex > (len(config.Servers)-1) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "A wrong del index!")
			return
		}

		config.Servers = append(config.Servers[:delslbindex], config.Servers[(delslbindex+1):]...)
		config.Delay = append(config.Delay[:delslbindex], config.Delay[(delslbindex+1):]...)

		writeconf()

		http.Redirect(w, r, "/manager", http.StatusTemporaryRedirect)

		return
	}

	if baseURL == "choosesingle" {

		if config.Mode != "single" {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Can't choose single index when not on single mode!")
			return
		}

		m, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(m) == 0 {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "Choose single backend url error")
			return
		}

		singleslbindex, err := strconv.Atoi(m["delslbindex"][0]) //复用了form的delslbindex

		if err != nil || singleslbindex > (len(config.Servers)-1) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "A wrong choose single index!")
			return
		}

		config.Theone = singleslbindex

		writeconf()

		http.Redirect(w, r, "/manager", http.StatusTemporaryRedirect)

		return

	}

	if baseURL == "dw" {
		str := "GET /dw HTTP/1.1\r\n"
		str += ("Host: " + r.Host + "\r\n")
		for k, v := range r.Header {
			str += (k + ": " + strings.Join(v, ",") + "\r\n")
		}
		str += "\r\n"
		log.Println(str)

		//log.Println("Getting ray access request...")
		hj, ok := w.(http.Hijacker)
		if !ok {
			log.Println("Hijacker error")
			http.Error(w, "webserver doesn't support hijacking", http.StatusInternalServerError)
			return
		}

		clientConn, _, err := hj.Hijack()
		if err != nil {
			log.Println("Hijacker Conn error")
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

	//log.Println("hj.hijacker is ok")
		defer clientConn.Close()

		server, err := net.Dial("tcp", "127.0.0.1:8080")
		if err != nil {
			log.Println("error to connect to v2ray:", err)
			return
		}

		server.Write([]byte(str))
		go io.Copy(server, clientConn)
		io.Copy(clientConn, server)

	}

	if len(config.Servers) > 0 {

		server := chooseServer(config.Servers, serverMethod)
		//	writeToLog("Healthy Server: " + server)
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
	switch config.Mode {
	case "random":
		for {
			count[method] = (count[method] + 1) % len(servers)
			if servers[count[method]] != "" && config.Delay[count[method]] != -1 {
				writeToLog("Chose random healthy server: " + servers[count[method]])
				return servers[count[method]]
			}
		}
	case "best":
		mindelay := config.Delay[0]
		minindex := 0
		//	slbdelay := delay[:len(config.Servers)]
		for index, val := range config.Delay {
			if mindelay > val && val > 0 {
				minindex = index
				mindelay = val
			}
		}
		writeToLog("Chose best healthy server: " + servers[minindex])
		return servers[minindex]
	case "single":
		writeToLog("Chose single server: " + servers[config.Theone])
		return servers[config.Theone]

	default:
		return "http://wechat.zhujq.ga"

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

// Could be improved but gets the job done
func reloadConfig(configFile string, conf chan Config, wg *sync.WaitGroup) {

	var oldConfig Config
	var t Config
	for {
		t = Parse(configFile)
		//	fmt.Println(reflect.DeepEqual(t, oldConfig))
		if reflect.DeepEqual(t.Servers, oldConfig.Servers) == false || t.Mode != oldConfig.Mode {
			conf <- t
			writeToLog("slb config is refreshed.")
			oldConfig = t
		}

		time.Sleep(600 * time.Second) //每10分钟刷新一次配置
	}
	close(conf)
	wg.Done()
	return
}

func refreshdelay(wg *sync.WaitGroup) {
	for {
		time.Sleep(120 * time.Second) //每2分钟刷新一次delay

		for i, wcserver := range config.Servers {
			t1 := time.Now()
			if HTTPGet(wcserver) == false {
				writeToLog(wcserver + " is not alive!")
				config.Delay[i] = -1 //设置延迟为-1表示不可达
			} else {
				t2 := time.Now()
				//	log.Println(wcserver + " delay is:" + strconv.Itoa(t2.Sub(t1).Milliseconds()))
				config.Delay[i] = int(t2.Sub(t1).Milliseconds()) //直接修改config全局变量的delay部分
			}
		}
		//	writeconf()
	}
	wg.Done()
	return
}

func launch(server *http.Server, wg *sync.WaitGroup) {
	writeToLog("Starting http slb service on port: " + server.Addr)
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
	wg.Add(3)

	count = make(map[int]int)

	configChannel := make(chan Config)

	if len(os.Args) > 1 {
		configFile = os.Args[1]
	}
	go reloadConfig(configFile, configChannel, &wg)
	go refreshdelay(&wg)

	go func() {
		for config = range configChannel {

			port := ":" + config.Port
			if port == ":" {
				port = port + "8080"
			}
			//		fmt.Println(server)
			/*	if server != nil {
					writeToLog("Server closing: " + server.Addr)
					//	fmt.Println("Server closing...")
					server.Close()
				}
			*/
			if server == nil {
				server = &http.Server{
					Addr:         port,
					ReadTimeout:  5 * time.Second,
					WriteTimeout: 10 * time.Second,
				}
				wg.Add(1)
				go launch(server, &wg)
			}
		}
		writeToLog("The SLB Web Service is Exited")
		wg.Done()
	}()

	wg.Wait()
}
