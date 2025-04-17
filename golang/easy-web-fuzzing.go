package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	domain        string
	wordlist      string
	concurrent    int
	depth         int
	httpsOnly     bool
	statusFilters string
	statusSet     map[int]struct{}
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
)

func init() {
	flag.StringVar(&domain, "d", "", "Target domain (ex: http://example.com)")
	flag.StringVar(&wordlist, "w", "wordlist.txt", "Path to your wordlist file, (usually under /usr/share/wordlists but idk)")
	flag.IntVar(&concurrent, "threads", 50, "Number of concurrent threads")
	flag.IntVar(&depth, "depth", 1, "Maximum recursion depth for subdomains")
	flag.BoolVar(&httpsOnly, "https-only", false, "Only check HTTPS URLs")
	flag.StringVar(&statusFilters, "status-filter", "", "Comma-separated list of HTTP status codes to display (e.g. 200,301)")
}

func main() {
	flag.Parse()

	if domain == "" {
		fmt.Println(colorRed + "[!] Please provide a ✌️VALID✌️ target domain " + colorReset)
		os.Exit(1)
	}

	statusSet = make(map[int]struct{})
	if statusFilters != "" {
		for _, s := range strings.Split(statusFilters, ",") {
			code, err := strconv.Atoi(strings.TrimSpace(s))
			if err == nil {
				statusSet[code] = struct{}{}
			}
		}
	}

	words, err := readWordlist(wordlist)
	if err != nil {
		fmt.Println(colorRed + fmt.Sprintf("[!] Failed to read wordlist, let's get back to the basics : %v", err) + colorReset)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrent)

	bruteforce(domain, 0, words, &wg, sem)

	wg.Wait()
}

func readWordlist(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var words []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		words = append(words, scanner.Text())
	}
	return words, scanner.Err()
}

func bruteforce(base string, level int, words []string, wg *sync.WaitGroup, sem chan struct{}) {
	if level >= depth {
		return
	}

	for _, sub := range words {
		fqdn := sub + "." + base
		wg.Add(1)
		sem <- struct{}{}

		go func(fqdn string) {
			defer wg.Done()
			if checkSubdomain(fqdn) {
				httpCheck(fqdn)
				bruteforce(fqdn, level+1, words, wg, sem)
			}
			<-sem
		}(fqdn)
	}
}

func checkSubdomain(fqdn string) bool {
	_, err := net.LookupHost(fqdn)
	if err == nil {
		fmt.Println(colorGreen + "[+] Domain Found: " + fqdn + colorReset)
		return true
	}
	return false
}

func httpCheck(fqdn string) {
	protocols := []string{"https://", "http://"}
	if httpsOnly {
		protocols = []string{"https://"}
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	titleRegex := regexp.MustCompile("(?i)<title>(.*?)</title>")

	for _, proto := range protocols {
		url := proto + fqdn
		req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
    
		if len(statusSet) > 0 {
			if _, ok := statusSet[resp.StatusCode]; !ok {
				continue
			}
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*10)) // read max 10KB, idk changeable , put your own damn number innit 
		if err != nil {
			continue
		}
		title := ""
		matches := titleRegex.FindSubmatch(body)
		if len(matches) > 1 {
			title = strings.TrimSpace(string(matches[1]))
		}
		fmt.Printf(colorBlue+"    [HTTP] %s => %d | %s"+colorReset+"\n", url, resp.StatusCode, title)
		break 
	}
}
