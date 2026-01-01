package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/tebeka/selenium"
)

var (
	numTests       int
	seleniumUrl    string
	pageUrl        string
	browserName    string
	browserVersion string
)

func init() {
	flag.IntVar(&numTests, "num-tests", 3, "Max tests to run in parallel")
	flag.StringVar(&seleniumUrl, "selenium-url", "http://192.168.1.103:32081/wd/hub", "Selenium URL to use")
	flag.StringVar(&pageUrl, "page-url", "https://aerokube.com/", "Page URL to open")
	flag.StringVar(&browserName, "browser-name", "chrome", "Browser to use")
	flag.StringVar(&browserVersion, "browser-version", "139.0", "Browser version to use")
	flag.Parse()
}

func main() {
	log.Printf("Using %s %s", browserName, browserVersion)
	var wg sync.WaitGroup
	for i := 1; i <= numTests; i++ {
		wg.Add(1)
		go runTest(&wg, i)
	}
	wg.Wait()
}

func runTest(wg *sync.WaitGroup, num int) {
	log.Printf("Running test %d", num)
	defer log.Printf("Test %d finished", num)
	defer wg.Done()
	selenium.HTTPClient = http.DefaultClient
	selenium.HTTPClient.Timeout = 10 * time.Minute
	caps := selenium.Capabilities{"browserName": browserName, "version": browserVersion}
	wd, err := selenium.NewRemote(caps, seleniumUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer wd.Quit()
	_ = wd.Get(pageUrl)
	time.Sleep(5 * time.Second)
	scrn, _ := wd.Screenshot()
	screenshotFile := fmt.Sprintf("screenshot%d.png", num)
	_ = ioutil.WriteFile(screenshotFile, scrn, os.ModePerm)
	log.Printf("Saved screenshot to %s", screenshotFile)
}
