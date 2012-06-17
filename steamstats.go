/*
 * Copyright 2012 Branimir Karadzic. All rights reserved.
 * License: http://www.opensource.org/licenses/BSD-2-Clause
 */

package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	permf         os.FileMode = 0660
	permd         os.FileMode = 02770
	daySeconds    int64       = 3600 * 24
	steamStatsUrl             = "http://store.steampowered.com/stats/"
	spanStart                 = `<span class="currentServers">`
	spanEnd                   = `</span>`
	nameStart                 = `<a class="gameLink" href="`
	nameEnd                   = `</a>`
)

var (
	flagInterval = flag.String("interval", "3600", "Snapshot inteval in seconds")
)

type GameInfo struct {
	Name    string
	Url     string
	Current int
	Peak    int
}

type Stats struct {
	Time  int64
	Games []GameInfo
}

func nextDay(t time.Time) int64 {

	_, zoneOffset := t.Zone()
	secs := t.Unix() + int64(zoneOffset)
	return secs + (daySeconds - (secs % daySeconds)) - int64(zoneOffset)
}

func openFile(filePath string) (*os.File, error) {

	dir, _ := path.Split(filePath)

	err := os.MkdirAll(dir, permd)
	if err != nil && err != syscall.EEXIST {
		return nil, err
	}

	return os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, permf)
}

func getFilePath(t time.Time) string {
	return "stats/" + t.Format("2006/01/2006-01-02") + ".json"
}

func parseStats(f *os.File, buf *bufio.Reader) {

	var (
		state int
		gi    GameInfo
		stats Stats
	)

	stats.Time = time.Now().Unix()

	for {
		data, _, err := buf.ReadLine()
		if err != nil {
			break
		}

		line := string(data)

		switch state {
		case 0:
			if strings.Contains(line, `<tr class="player_count_row" style="`) {
				state++
			}

		case 1, 2:
			start := strings.Index(line, spanStart)
			if start != -1 {
				start += len(spanStart)
				end := strings.Index(line[start:], spanEnd)
				if end != -1 {
					num, err := strconv.Atoi(strings.Replace(line[start:start+end], ",", "", -1))
					if err == nil {
						if state == 1 {
							gi.Current = num
						} else {
							gi.Peak = num
						}
					}
				}

				state++
			}

		case 3:
			start := strings.Index(line, nameStart)
			if start != -1 {
				start += len(nameStart)
				end := strings.Index(line[start:], `">`)
				if end != -1 {
					gi.Url = line[start : start+end]
					start += end + 2
					end = strings.Index(line[start:], `</a>`)
					if end != -1 {
						gi.Name = line[start : start+end]
					}
				}

				stats.Games = append(stats.Games, gi)

				state = 0
			}
		}
	}

	data, _ := json.MarshalIndent(stats, "", "\t")
	f.Write(data)
}

func main() {

	flag.Parse()

	local := time.Now()
	next := time.Unix(nextDay(local), 0)

	filePath := getFilePath(local)
	f, err := openFile(filePath)
	if err != nil {
		fmt.Printf("Failed to create/open file ", filePath)
		os.Exit(1)
	}

	interval, err := strconv.ParseInt(*flagInterval, 10, 64)
	if err != nil {
		interval = 3600
	}

	for {

		chNow := time.After(time.Duration(interval * 1e9))

		resp, err := http.Get(steamStatsUrl)

		if err == nil {

			data, _ := ioutil.ReadAll(resp.Body)
			buf := bufio.NewReader(bytes.NewReader(data))

			if local.Unix() >= next.Unix() {

				f.Close()

				filePath = getFilePath(local)
				next = time.Unix(nextDay(local), 0)
				f, err = openFile(filePath)
			}

			parseStats(f, buf)
		}

		local = time.Unix((<-chNow).Unix(), 0)
	}

	f.Close()
}
