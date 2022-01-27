package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	mpris2 "github.com/arafatamim/mpris2client"
	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"
	flag "github.com/spf13/pflag"
)

// Various paths and values to use elsewhere.
const (
	SOCK         = "/tmp/waybar-mpris.sock"
	LOGFILE      = "/tmp/waybar-mpris.log"
	OUTFILE      = "/tmp/waybar-mpris.out"      // Used for sharing waybar output when args are the same.
	DATAFILE     = "/tmp/waybar-mpris.data.out" // Used for sharing "\n"-separated player data between instances when args are different.
	POLLINTERVAL = 1
)

// Mostly default values for flag options.
var (
	PLAY      = "▶"
	PAUSE     = ""
	SEP       = " - "
	ORDER     = "SYMBOL:ARTIST:ALBUM:TITLE:POSITION"
	AUTOFOCUS = false
	// Available commands that can be sent to running instances.
	COMMANDS                          = []string{"player-next", "player-prev", "next", "prev", "toggle", "list"}
	POSFORMAT                         = "(P/L)"
	PADZERO                           = true
	INTERPOLATE                       = false
	REPLACE                           = false
	isSharing                         = false
	isDataSharing                     = false
	isPolling                         = true
	WRITER                  io.Writer = os.Stdout
	SHAREWRITER, DATAWRITER io.Writer
)

const (
	cPlayerNext     = "pn"
	cPlayerPrev     = "pp"
	cNext           = "mn"
	cPrev           = "mp"
	cToggle         = "mt"
	cList           = "ls"
	cShare          = "sh"
	cPreShare       = "ps"
	cDataShare      = "ds"
	rSuccess        = "sc"
	rInvalidCommand = "iv"
	rFailed         = "fa"
)

func stringToCmd(str string) string {
	switch str {
	case "player-next":
		return cPlayerNext
	case "player-prev":
		return cPlayerPrev
	case "next":
		return cNext
	case "prev":
		return cPrev
	case "toggle":
		return cToggle
	case "list":
		return cList
	case "share":
		return cShare
	case "data-share":
		return cDataShare
	case "pre-share":
		return cPreShare
	}
	return ""
}

func formatString(text string) string {
	s := strings.ReplaceAll(text, "\"", "\\\"")
	s = strings.ReplaceAll(s, "&", "&amp;")
	return s
}

// length-µS\nposition-µS\nplaying (0 or 1)\nartist\nalbum\ntitle\nplayer\n
func fromData(p *player, cmd string) {
	p.Duplicate = true
	values := make([]string, 7)
	prev := 0
	current := 0
	for i := range cmd {
		if current == len(values) {
			break
		}
		if cmd[i] == '\n' {
			values[current] = cmd[prev:i]
			prev = i + 1
			current++
		}
	}
	l, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil {
		l = -1
	}
	p.Length = int(l) / 1000000
	pos, err := strconv.ParseInt(values[1], 10, 64)
	if err != nil {
		pos = -1
	}
	p.Position = pos

	if values[2] == "1" {
		p.Playing = true
	} else {
		p.Playing = false
	}
	p.Artist = values[3]
	p.Album = values[4]
	p.Title = values[5]
	p.Name = values[6]
}

func toData(p *player) (cmd string) {
	cmd += strconv.FormatInt(int64(p.Length*1000000), 10) + "\n"
	cmd += strconv.FormatInt(p.Position, 10) + "\n"
	if p.Playing {
		cmd += "1"
	} else {
		cmd += "0"
	}
	cmd += "\n"
	cmd += p.Artist + "\n"
	cmd += p.Album + "\n"
	cmd += p.Title + "\n"
	cmd += p.Name + "\n"
	return
}

type player struct {
	*mpris2.Player
	Duplicate bool
}

func formatSeconds(seconds int) string {
	minutes := int(seconds / 60)
	seconds -= int(minutes * 60)
	hours := int(minutes / 60)

	if hours == 0 {
		if PADZERO {
			return fmt.Sprintf("%02d:%02d", minutes, seconds)
		} else {
			return fmt.Sprintf("%d:%02d", minutes, seconds)
		}
	} else {
		if PADZERO {
			return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
		} else {
			return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
		}
	}
}

func formatDuration(position int, length int) string {
	posStr := formatSeconds(position)
	lenStr := formatSeconds(length)

	str := strings.ReplaceAll(POSFORMAT, "P", posStr)
	str = strings.ReplaceAll(str, "L", lenStr)

	return str
}

// JSON returns json for waybar to consume.
func playerJSON(p *player) string {
	symbol := PLAY
	out := "{\"class\":\""
	if p.Playing {
		symbol = PAUSE
		out += "playing"
	} else {
		out += "paused"
	}
	var pos string
	if !p.Duplicate {
		pos = p.StringPosition(" - ")
		if pos != "" {
			pos = formatDuration(int(p.Position/1000000), p.Length)
		}
	} else {
		pos = formatDuration(int(p.Position/1000000), p.Length)
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
			if pos != "" {
				items = append(items, pos)
			} else {
				isPolling = false
			}
		case "PLAYER":
			if p.Name != "" {
				items = append(items, p.Name)
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
			right = " "
		}
		text += v + right
	}
	out += "\",\"text\":\"" + formatString(text) + "\","
	out += "\"tooltip\":\"" + formatString(p.Title) + "\\n"
	if p.Artist != "" {
		out += "by " + formatString(p.Artist) + "\\n"
	}
	if p.Album != "" {
		out += "from " + formatString(p.Album) + "\\n"
	}
	out += "(" + p.Name + ")\"}"
	return out
}

type players struct {
	mpris2 *mpris2.Mpris2
}

func (pl *players) JSON() string {
	if len(pl.mpris2.List) != 0 {
		return playerJSON(&player{pl.mpris2.List[pl.mpris2.Current], false})
	}
	return "{}"
}

func (pl *players) Next() { pl.mpris2.List[pl.mpris2.Current].Next() }

func (pl *players) Prev() { pl.mpris2.List[pl.mpris2.Current].Previous() }

func (pl *players) Toggle() { pl.mpris2.List[pl.mpris2.Current].Toggle() }

func execCommand(cmd string) {
	conn, err := net.Dial("unix", SOCK)
	if err != nil {
		log.Fatalln("Couldn't dial:", err)
	}
	shortCmd := stringToCmd(cmd)
	_, err = conn.Write([]byte(shortCmd))
	if err != nil {
		log.Fatalln("Couldn't send command")
	}
	fmt.Println("Sent.")
	if cmd == "list" {
		buf := make([]byte, 512)
		nr, err := conn.Read(buf)
		if err != nil {
			log.Fatalln("Couldn't read response.")
		}
		response := string(buf[0:nr])
		fmt.Println("Response:")
		fmt.Printf(response)
	}
	os.Exit(0)
}

func duplicateOutput() error {
	// Print to stderr to avoid errors from waybar
	os.Stderr.WriteString("waybar-mpris is already running. This instance will clone its output.")

	conn, err := net.Dial("unix", SOCK)
	if err != nil {
		return err
	}
	_, err = conn.Write([]byte(cPreShare))
	if err != nil {
		log.Fatalf("Couldn't send command: %v", err)
		return err
	}
	buf := make([]byte, 512)
	nr, err := conn.Read(buf)
	if err != nil {
		log.Fatalf("Couldn't read response: %v", err)
		return err
	}
	argString := ""
	for _, arg := range os.Args {
		argString += arg + "|"
	}
	conn.Close()
	conn, err = net.Dial("unix", SOCK)
	if err != nil {
		return err
	}
	if string(buf[0:nr]) == argString {
		// Tell other instance to share output in OUTFILE
		_, err := conn.Write([]byte(cShare))
		if err != nil {
			log.Fatalf("Couldn't send command: %v", err)
		}
		buf = make([]byte, 2)
		nr, err := conn.Read(buf)
		if err != nil {
			log.Fatalf("Couldn't read response: %v", err)
		}
		if resp := string(buf[0:nr]); resp == rSuccess {
			// t, err := tail.TailFile(OUTFILE, tail.Config{
			// 	Follow:    true,
			// 	MustExist: true,
			// 	Logger:    tail.DiscardingLogger,
			// })
			// if err == nil {
			// 	for line := range t.Lines {
			// 		fmt.Println(line.Text)
			// 	}
			// }
			f, err := os.Open(OUTFILE)
			if err != nil {
				log.Fatalf("Failed to open \"%s\": %v", OUTFILE, err)
			}
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Fatalf("Failed to start watcher: %v", err)
			}
			defer watcher.Close()
			err = watcher.Add(OUTFILE)
			if err != nil {
				log.Fatalf("Failed to watch file: %v", err)
			}
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						log.Printf("Watcher failed: %v", err)
						return err
					}
					if event.Op&fsnotify.Write == fsnotify.Write {
						l, err := io.ReadAll(f)
						if err != nil {
							log.Printf("Failed to read file: %v", err)
							return err
						}
						str := string(l)
						// Trim extra newline is necessary
						if str[len(str)-2:] == "\n\n" {
							fmt.Print(str[:len(str)-1])
						} else {
							fmt.Print(str)
						}
						f.Seek(0, 0)
					}
				}
			}
		}
	} else {
		_, err := conn.Write([]byte(cDataShare))
		if err != nil {
			log.Fatalf("Couldn't send command: %v", err)
		}
		buf = make([]byte, 2)
		nr, err := conn.Read(buf)
		if err != nil {
			log.Fatalf("Couldn't read response: %v", err)
		}
		if resp := string(buf[0:nr]); resp == rSuccess {
			f, err := os.Open(DATAFILE)
			if err != nil {
				log.Fatalf("Failed to open \"%s\": %v", DATAFILE, err)
			}
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				log.Fatalf("Failed to start watcher: %v", err)
			}
			defer watcher.Close()
			err = watcher.Add(DATAFILE)
			if err != nil {
				log.Fatalf("Failed to watch file: %v", err)
			}
			p := &player{
				&mpris2.Player{},
				true,
			}
			for {
				select {
				case event, ok := <-watcher.Events:
					if !ok {
						log.Printf("Watcher failed: %v", err)
						return err
					}
					if event.Op&fsnotify.Write == fsnotify.Write {
						l, err := io.ReadAll(f)
						if err != nil {
							log.Printf("Failed to read file: %v", err)
							return err
						}
						str := string(l)
						fromData(p, str)
						fmt.Fprintln(WRITER, playerJSON(p))
						f.Seek(0, 0)
					}
				}
			}
		}
	}
	return nil
}

func listenForCommands(players *players) {
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
		buf := make([]byte, 2)
		nr, err := con.Read(buf)
		if err != nil {
			log.Println("Couldn't read:", err)
			continue
		}
		command := string(buf[0:nr])
		switch command {
		case cPlayerNext:
			length := len(players.mpris2.List)
			if length != 1 {
				if players.mpris2.Current < uint(length-1) {
					players.mpris2.Current++
				} else {
					players.mpris2.Current = 0
				}
				players.mpris2.Refresh()
			}
		case cPlayerPrev:
			length := len(players.mpris2.List)
			if length != 1 {
				if players.mpris2.Current != 0 {
					players.mpris2.Current--
				} else {
					players.mpris2.Current = uint(length - 1)
				}
				players.mpris2.Refresh()
			}
		case cNext:
			players.Next()
		case cPrev:
			players.Prev()
		case cToggle:
			players.Toggle()
		case cList:
			con.Write([]byte(players.mpris2.String()))
		case cDataShare:
			if !isDataSharing {
				f, err := os.OpenFile(DATAFILE, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
				defer f.Close()
				if err != nil {
					fmt.Fprintf(con, "Failed: %v", err)
				}
				DATAWRITER = dataWrite{
					emptyEveryWrite{file: f},
					players,
				}
				if isSharing {
					WRITER = io.MultiWriter(os.Stdout, SHAREWRITER, DATAWRITER)
				} else {
					WRITER = io.MultiWriter(os.Stdout, DATAWRITER)
				}
				isDataSharing = true
			}
			fmt.Fprint(con, rSuccess)
		case cShare:
			if !isSharing {
				f, err := os.OpenFile(OUTFILE, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
				defer f.Close()
				if err != nil {
					fmt.Fprintf(con, "Failed: %v", err)
				}
				SHAREWRITER = emptyEveryWrite{file: f}
				if isDataSharing {
					WRITER = io.MultiWriter(SHAREWRITER, DATAWRITER, os.Stdout)
				} else {
					WRITER = io.MultiWriter(SHAREWRITER, os.Stdout)
				}
				isSharing = true
			}
			fmt.Fprint(con, rSuccess)
		/* Prior to sharing, the first instance sends its os.Args.
		If the second instances args are different, the first sends the raw data (artist, album, etc.)
		If they are the same, the first instance just sends its output and the second prints it. */
		case cPreShare:
			out := ""
			for _, arg := range os.Args {
				out += arg + "|"
			}
			con.Write([]byte(out))
		default:
			fmt.Println("Invalid command")
		}
		con.Close()
	}
}

type dataWrite struct {
	emptyEveryWrite
	Players *players
}

func (w dataWrite) Write(p []byte) (n int, err error) {
	line := toData(&player{w.Players.mpris2.List[w.Players.mpris2.Current], true})
	_, err = w.emptyEveryWrite.Write([]byte(line))
	n = len(p)
	return
}

type emptyEveryWrite struct {
	file *os.File
}

func (w emptyEveryWrite) Write(p []byte) (n int, err error) {
	n = len(p)
	// Set new size in case previous data was longer and would leave garbage at the end of the file.
	err = w.file.Truncate(int64(n))
	if err != nil {
		return 0, err
	}
	offset, err := w.file.Seek(0, 0)
	if err != nil {
		return 0, err
	}
	_, err = w.file.WriteAt(p, offset)
	return
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
	flag.StringVar(&ORDER, "order", ORDER, "Element order. An extra \"PLAYER\" element is also available.")
	flag.BoolVar(&AUTOFOCUS, "autofocus", AUTOFOCUS, "Auto switch to currently playing music players.")
	flag.StringVar(&POSFORMAT, "position-format", POSFORMAT, "Format string for track duration, where P is current position, L is the track length.")
	flag.BoolVar(&PADZERO, "pad-zero", PADZERO, "Apply zero padding to track duration.")
	flag.BoolVar(&INTERPOLATE, "interpolate", INTERPOLATE, "Interpolate track position (helpful for players that don't update regularly, e.g mpDris2)")
	flag.BoolVar(&REPLACE, "replace", REPLACE, "replace existing waybar-mpris if found. When false, new instance will clone the original instances output.")
	var command string
	flag.StringVar(&command, "send", "", "send command to already runnning waybar-mpris instance. (options: "+strings.Join(COMMANDS, "/")+")")

	flag.Parse()
	os.Stderr = logfile

	if command != "" {
		execCommand(command)
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
			// When waybar-mpris is already running, we attach to its output instead of launching a whole new instance.
		} else if err := duplicateOutput(); err != nil {
			os.Stderr.WriteString("Couldn't dial socket, deleting instead: " + err.Error())
			os.Remove(SOCK)
			os.Remove(OUTFILE)
		}
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		log.Fatalln("Error connecting to DBus:", err)
	}
	players := &players{
		mpris2: mpris2.NewMpris2(conn, INTERPOLATE, POLLINTERVAL, AUTOFOCUS),
	}
	players.mpris2.Reload()
	players.mpris2.Sort()
	lastLine := ""
	go listenForCommands(players)
	go players.mpris2.Listen()
	if isPolling {
		go func() {
			for {
				time.Sleep(POLLINTERVAL * time.Second)
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
