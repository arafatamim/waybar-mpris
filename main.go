package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"time"

	mpris2 "github.com/arafatamim/mpris2client"
	"github.com/godbus/dbus/v5"
	"github.com/hpcloud/tail"
	flag "github.com/spf13/pflag"
)

// Various paths and values to use elsewhere.
const (
	SOCK    = "/tmp/waybar-mpris.sock"
	LOGFILE = "/tmp/waybar-mpris.log"
	OUTFILE = "/tmp/waybar-mpris.out"
	POLL    = 1
)

// Mostly default values for flag options.
var (
	PLAY      = "▶"
	PAUSE     = ""
	SEP       = " - "
	ORDER     = "SYMBOL:ARTIST:ALBUM:TITLE:POSITION"
	AUTOFOCUS = false
	// Available commands that can be sent to running instances.
	COMMANDS              = []string{"player-next", "player-prev", "next", "prev", "toggle", "list"}
	SHOW_POS              = false
	INTERPOLATE           = false
	REPLACE               = false
	WRITER      io.Writer = os.Stdout
)

// JSON returns json for waybar to consume.
func playerJSON(p *mpris2.Player) string {
	data := map[string]string{}

	// SYMBOL
	symbol := PAUSE
	data["class"] = "paused"
	if p.Playing {
		symbol = PLAY
		data["class"] = "playing"
	}

	// POSITION
	var pos string
	if SHOW_POS {
		pos = p.StringPosition(" - ")
		// if pos != "" {
		// 	pos = "(" + pos + ")"
		// }
	}

	var items []string
	order := strings.Split(ORDER, ":")
	for _, v := range order {
		switch v {
		case "SYMBOL":
			items = append(items, symbol)
		case "ARTIST":
			if p.Artist != "" {
				items = append(items, p.Artist)
			}
		case "ALBUM":
			if p.Album != "" {
				items = append(items, p.Album)
			}
		case "TITLE":
			if p.Title != "" {
				items = append(items, p.Title)
			}
		case "POSITION":
			if pos != "" && SHOW_POS {
				items = append(items, pos)
			}
		}
	}
	if len(items) == 0 {
		return "{}"
	}
	text := ""
	for i, v := range items {
		right := ""
		if (v == symbol || v == pos) && i != len(items)-1 {
			right = " "
		} else if i != len(items)-1 && items[i+1] != symbol && items[i+1] != pos {
			right = SEP
		} else {
			right = ""
		}
		text += v + right
	}

	data["tooltip"] = fmt.Sprintf(
		"%s\nby %s\n",
		strings.ReplaceAll(p.Title, "&", "&amp;"),
		strings.ReplaceAll(p.Artist, "&", "&amp;"))
	if p.Artist == "" && p.Title == "" {
		data["tooltip"] = "" // Empty tooltip if not title and artist
	}
	if p.Album != "" {
		data["tooltip"] += "from " + strings.ReplaceAll(p.Album, "&", "&amp;") + "\n"
	}
	data["tooltip"] += "(" + p.Name + ")"
	data["text"] = text
	out, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(out)
}

type players struct {
	mpris2 *mpris2.Mpris2
}

func (pl *players) JSON() string {
	if len(pl.mpris2.List) != 0 {
		return playerJSON(pl.mpris2.List[pl.mpris2.Current])
	}
	return "{}"
}

func (pl *players) Next() { pl.mpris2.List[pl.mpris2.Current].Next() }

func (pl *players) Prev() { pl.mpris2.List[pl.mpris2.Current].Previous() }

func (pl *players) Toggle() { pl.mpris2.List[pl.mpris2.Current].Toggle() }

func (pl *players) Stop() {
	pl.mpris2.List[pl.mpris2.Current].Stop()
}

func main() {
	logfile, err := os.OpenFile(LOGFILE, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0666)
	if err != nil {
		log.Fatalf("Couldn't open %s for writing: %s", LOGFILE, err)
	}
	mw := io.MultiWriter(logfile, os.Stdout)
	log.SetOutput(mw)
	flag.StringVar(&PLAY, "play", PLAY, "Play symbol/text to use.")
	flag.StringVar(&PAUSE, "pause", PAUSE, "Pause symbol/text to use.")
	flag.StringVar(&SEP, "separator", SEP, "Separator string to use between artist, album, and title.")
	flag.StringVar(&ORDER, "order", ORDER, "Element order.")
	flag.BoolVar(&AUTOFOCUS, "autofocus", AUTOFOCUS, "Auto switch to currently playing music players.")
	flag.BoolVar(&SHOW_POS, "position", SHOW_POS, "Show current position between brackets, e.g (04:50/05:00)")
	flag.BoolVar(&INTERPOLATE, "interpolate", INTERPOLATE, "Interpolate track position (helpful for players that don't update regularly, e.g mpDris2)")
	flag.BoolVar(&REPLACE, "replace", REPLACE, "replace existing waybar-mpris if found. When false, new instance will clone the original instances output.")
	var command string
	flag.StringVar(&command, "send", "", "send command to already runnning waybar-mpris instance. (options: "+strings.Join(COMMANDS, "/")+")")

	flag.Parse()
	os.Stderr = logfile

	if command != "" {
		conn, err := net.Dial("unix", SOCK)
		if err != nil {
			log.Fatalln("Couldn't dial:", err)
		}
		_, err = conn.Write([]byte(command))
		if err != nil {
			log.Fatalln("Couldn't send command")
		}
		fmt.Println("Sent.")
		if command == "list" {
			buf := make([]byte, 512)
			nr, err := conn.Read(buf)
			if err != nil {
				log.Fatalln("Couldn't read response.")
			}
			response := string(buf[0:nr])
			fmt.Println("Response:")
			fmt.Print(response)
		}
		os.Exit(0)
	}
	// fmt.Println("New array", players)
	// Start command listener
	if _, err := os.Stat(SOCK); err == nil {
		if REPLACE {
			fmt.Printf("Socket %s already exists, this could mean waybar-mpris is already running.\nStarting this instance will overwrite the file, possibly stopping other instances from accepting commands.\n", SOCK)
			var input string
			ignoreChoice := false
			fmt.Printf("Continue? [y/n]: ")
			go func() {
				fmt.Scanln(&input)
				if strings.Contains(input, "y") && !ignoreChoice {
					os.Remove(SOCK)
				}
			}()
			time.Sleep(5 * time.Second)
			if input == "" {
				fmt.Printf("\nRemoving due to lack of input.\n")
				ignoreChoice = true
				// os.Remove(SOCK)
			}
		} else if conn, err := net.Dial("unix", SOCK); err == nil {
			// When waybar-mpris is already running, we attach to its output instead of launching a whole new instance.
			// Print to stderr to avoid errors from waybar
			os.Stderr.WriteString("waybar-mpris is already running. This instance will clone its output.")
			if err != nil {
				log.Fatalln("Couldn't dial:", err)
			}
			_, err = conn.Write([]byte("share"))
			if err != nil {
				log.Fatalln("Couldn't send command")
			}
			buf := make([]byte, 512)
			nr, err := conn.Read(buf)
			if err != nil {
				log.Fatalln("Couldn't read response.")
			}
			if string(buf[0:nr]) == "success" {
				t, err := tail.TailFile(OUTFILE, tail.Config{
					Follow:    true,
					MustExist: true,
					Logger:    tail.DiscardingLogger,
				})
				if err == nil {
					for line := range t.Lines {
						fmt.Println(line.Text)
					}
				}
			}
		} else {
			os.Remove(SOCK)
			os.Remove(OUTFILE)
		}
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalln("Error connecting to DBus:", err)
	}
	players := &players{
		mpris2: mpris2.NewMpris2(conn, INTERPOLATE, POLL, AUTOFOCUS),
	}
	players.mpris2.Reload()
	players.mpris2.Sort()
	lastLine := ""
	go func() {
		listener, err := net.Listen("unix", SOCK)
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)
		go func() {
			<-c
			os.Remove(OUTFILE)
			os.Remove(SOCK)
			os.Exit(1)
		}()
		if err != nil {
			log.Fatalf("Couldn't establish socket connection at %s (error %s)\n", SOCK, err)
		}
		defer func() {
			listener.Close()
			os.Remove(SOCK)
		}()
		for {
			con, err := listener.Accept()
			if err != nil {
				log.Println("Couldn't accept:", err)
				continue
			}
			buf := make([]byte, 512)
			nr, err := con.Read(buf)
			if err != nil {
				log.Println("Couldn't read:", err)
				continue
			}
			command := string(buf[0:nr])
			switch command {
			case "player-next":
				length := len(players.mpris2.List)
				if length != 1 {
					if players.mpris2.Current < uint(length-1) {
						players.mpris2.Current++
					} else {
						players.mpris2.Current = 0
					}
					players.mpris2.Refresh()
				}
			case "player-prev":
				length := len(players.mpris2.List)
				if length != 1 {
					if players.mpris2.Current != 0 {
						players.mpris2.Current--
					} else {
						players.mpris2.Current = uint(length - 1)
					}
					players.mpris2.Refresh()
				}
			case "next":
				players.Next()
			case "prev":
				players.Prev()
			case "toggle":
				players.Toggle()
			case "list":
				con.Write([]byte(players.mpris2.String()))
			case "share":
				out, err := os.Create(OUTFILE)
				if err != nil {
					con.Write([]byte(fmt.Sprintf("Failed: %s", err)))
				}
				WRITER = io.MultiWriter(os.Stdout, out)
				con.Write([]byte("success"))
			default:
				fmt.Println("Invalid command")
			}
		}
	}()
	go players.mpris2.Listen()
	if SHOW_POS {
		go func() {
			for {
				time.Sleep(POLL * time.Second)
				if len(players.mpris2.List) != 0 {
					if players.mpris2.List[players.mpris2.Current].Playing {
						go fmt.Fprintln(WRITER, players.JSON())
					}
				}
			}
		}()
	}
	fmt.Fprintln(WRITER, players.JSON())
	for v := range players.mpris2.Messages {
		if v.Name == "refresh" {
			if AUTOFOCUS {
				players.mpris2.Sort()
			}
			if l := players.JSON(); l != lastLine {
				lastLine = l
				fmt.Fprintln(WRITER, l)
			}
		}
	}
	players.mpris2.Refresh()
}
