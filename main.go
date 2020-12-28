package main

import (
	"bytes"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/dustin/go-humanize"
	"github.com/pyed/tailer"
	"gopkg.in/machsix/transmission.v1"
	tgbotapi "gopkg.in/telegram-bot-api.v4"
)

var userPrefs = []language.Tag{
	language.Make("en"), // Swiss German
	language.Make("ru"), // French
}

var serverLangs = []language.Tag{
	language.English, // en fallback
	language.Russian, // ru
}

var matcher = language.NewMatcher(serverLangs)

const (
	VERSION = "v1.4"

	HELP = `
	**/setdir**
	Set relative directory for download

	**/getdir**
	Get current download directory
	
	**/list**
	Lists all the torrents, takes an optional argument which is a query to list only torrents that has a tracker matches the query, or some of it.

	**/head**
	Lists the first n number of torrents, n defaults to 5 if no argument is provided.

	**/tail**
	Lists the last n number of torrents, n defaults to 5 if no argument is provided.

	**/down**
	Lists torrents with the status of _Downloading_ or in the queue to download.

	**/seeding**
	Lists torrents with the status of _Seeding_ or in the queue to seed.

	**/paused**
	Lists _Paused_ torrents.


	**/checking**
	Lists torrents with the status of _Verifying_ or in the queue to verify.

	**/active**
	Lists torrents that are actively uploading or downloading.

	**/errors**
	Lists torrents with with errors along with the error message.

	**/sort**
	Manipulate the sorting of the aforementioned commands. Call it without arguments for more.

	**/trackers**
	Lists all the trackers along with the number of torrents.

	**/add**
	Takes one or many URLs or magnets to add them. You can send a ".torrent" file via Telegram to add it.

	**/search**
	Takes a query and lists torrents with matching names.

	**/latest**
	Lists the newest n torrents, n defaults to 5 if no argument is provided.

	**/info**
	Takes one or more torrent's IDs to list more info about them.

	**/stop**
	Takes one or more torrent's IDs to stop them, or _all_ to stop all torrents.

	**/start**
	Takes one or more torrent's IDs to start them, or _all_ to start all torrents.

	**/check**
	Takes one or more torrent's IDs to verify them, or _all_ to verify all torrents.

	**/del**
	Takes one or more torrent's IDs to delete them.

	**/deldata**
	Takes one or more torrent's IDs to delete them and their data.

	**/stats**
	Shows Transmission's stats.

	**/speed**
	Shows the upload and download speeds.

	**/count**
	Shows the torrents counts per status.

	**/help**
	Shows this help message.

	**/version**
	Shows version numbers.

	- report any issues [here](https://github.com/machsix/transmission-telegram)
	`
	HELP_RU = `
	**/setdir**
	Set relative directory for download

	**/getdir**
	Get current download directory
	
	**/list**
	Lists all the torrents, takes an optional argument which is a query to list only torrents that has a tracker matches the query, or some of it.

	**/head**
	Lists the first n number of torrents, n defaults to 5 if no argument is provided.

	**/tail**
	Lists the last n number of torrents, n defaults to 5 if no argument is provided.

	**/down**
	Lists torrents with the status of _Downloading_ or in the queue to download.

	**/seeding**
	Lists torrents with the status of _Seeding_ or in the queue to seed.

	**/paused**
	Lists _Paused_ torrents.


	**/checking**
	Lists torrents with the status of _Verifying_ or in the queue to verify.

	**/active**
	Lists torrents that are actively uploading or downloading.

	**/errors**
	Lists torrents with with errors along with the error message.

	**/sort**
	Manipulate the sorting of the aforementioned commands. Call it without arguments for more.

	**/trackers**
	Lists all the trackers along with the number of torrents.

	**/add**
	Takes one or many URLs or magnets to add them. You can send a ".torrent" file via Telegram to add it.

	**/search**
	Takes a query and lists torrents with matching names.

	**/latest**
	Lists the newest n torrents, n defaults to 5 if no argument is provided.

	**/info**
	Takes one or more torrent's IDs to list more info about them.

	**/stop**
	Takes one or more torrent's IDs to stop them, or _all_ to stop all torrents.

	**/start**
	Takes one or more torrent's IDs to start them, or _all_ to start all torrents.

	**/check**
	Takes one or more torrent's IDs to verify them, or _all_ to verify all torrents.

	**/del**
	Takes one or more torrent's IDs to delete them.

	**/deldata**
	Takes one or more torrent's IDs to delete them and their data.

	**/stats**
	Shows Transmission's stats.

	**/speed**
	Shows the upload and download speeds.

	**/count**
	Shows the torrents counts per status.

	**/help**
	Shows this help message.

	**/version**
	Shows version numbers.

	- report any issues [here](https://github.com/machsix/transmission-telegram)
	`
)

var (

	// flags
	BotToken     string
	Masters      masterSlice
	RPCURL       string
	Username     string
	Password     string
	LogFile      string
	TransLogFile string // Transmission log file
	NoLive       bool
	PrivateOnly  bool
	DownLoadDir  string
	RootDir      string
	DownloadDirs dirsSlice
	downsDirs    string
	DefaultDir   string
	langTag      string
	// transmission
	Client *transmission.TransmissionClient

	// telegram
	Bot     *tgbotapi.BotAPI
	Updates <-chan tgbotapi.Update

	// chatID will be used to keep track of which chat to send completion notifictions.
	chatID int64

	// logging
	logger = log.New(os.Stdout, "", log.LstdFlags)

	// interval in seconds for live updates, affects: "active", "info", "speed", "head", "tail"
	interval time.Duration = 5
	// duration controls how many intervals will happen
	duration = 10

	// since telegram's markdown can't be escaped, we have to replace some chars
	// affects only markdown users: info, active, head, tail
	mdReplacer = strings.NewReplacer("*", "•",
		"[", "(",
		"]", ")",
		"_", "-",
		"`", "'")
)

// we need a type for masters for the flag package to parse them as a slice
type masterSlice []string
type dirsSlice []string

// String is mandatory functions for the flag package
func (masters *masterSlice) String() string {
	return fmt.Sprintf("%s", *masters)
}

// Set is mandatory functions for the flag package
func (masters *masterSlice) Set(master string) error {
	*masters = append(*masters, strings.ToLower(master))
	return nil
}

// Contains takes a string and return true of masterSlice has it
func (masters masterSlice) Contains(master string) bool {
	master = strings.ToLower(master)
	for i := range masters {
		if masters[i] == master {
			return true
		}
	}
	return false
}

////Для диров
// String is mandatory functions for the flag package
func (dirs *dirsSlice) String() string {
	return fmt.Sprintf("%s", *dirs)
}

// Set is mandatory functions for the flag package
func (dirs *dirsSlice) Set(dir string) error {
	*dirs = append(*dirs, dir)
	return nil
}

// Contains takes a string and return true of dirsSlice has it
func (dirs dirsSlice) Contains(dir string) bool {
	//master = strings.ToLower(master)
	for i := range dirs {
		if dirs[i] == dir {
			return true
		}
	}
	return false
}

// init flags
func init() {
	// Пробы с языком
	message.SetString(language.Russian, "In %v people speak %v.", "В %v говорят на %v.")
	message.SetString(language.Russian, "Usage: transmission-telegram <-token=TOKEN> <-master=@tuser> [-master=@yuser2] [-url=http://] [-username=user] [-password=pass] [-dir=downloadDirectory]\n\n",
		"Используйте: transmission-telegram <-token=TOKEN> <-master=@tuser> [-master=@yuser2] [-url=http://] [-username=user] [-password=pass] [-dir=downloadDirectory]\n\n")
	message.SetString(language.Russian, "Error: Mandatory argument missing! (-token or -master)\n\n", "Ошибка: Пропущен обязательный аргумент! (-token или -master)\n\n")

	/*fr := language.French
	region, _ := fr.Region()
	for _, tag := range []string{"en", "ru"} {
		p := message.NewPrinter(language.Make(tag))

		p.Printf("In %v people speak %v.", display.Region(region), display.Language(fr))
		p.Println()
	}*/
	// define arguments and parse them.
	flag.StringVar(&BotToken, "token", "", "Telegram bot token, Can be passed via environment variable 'TT_BOTT'")
	flag.Var(&Masters, "master", "Your telegram handler, So the bot will only respond to you. Can specify more than one")
	flag.StringVar(&RPCURL, "url", "http://localhost:9091/transmission/rpc", "Transmission RPC URL")
	flag.StringVar(&Username, "username", "", "Transmission username")
	flag.StringVar(&Password, "password", "", "Transmission password")
	flag.StringVar(&LogFile, "logfile", "", "Send logs to a file")
	flag.StringVar(&TransLogFile, "transmission-logfile", "", "Open transmission logfile to monitor torrents completion")
	flag.BoolVar(&NoLive, "no-live", false, "Don't edit and update info after sending")
	flag.BoolVar(&PrivateOnly, "private", false, "Only receive message from private chat")
	flag.StringVar(&DefaultDir, "defdir", "Downloads", "Default download dir")
	flag.StringVar(&RootDir, "rootdir", "/volume1/SHARE_DRIVE", "Root of download directory")
	flag.StringVar(&langTag, "lang", "ru", "Set bot language")
	//flag.Var(&DownloadDirs, "downloaddirs", "Set download dirs in root dir")
	flag.StringVar(&downsDirs, "downloaddirs", "", "Set download dirs in root dir")

	prn := message.NewPrinter(language.Make(langTag))

	// set the usage message
	flag.Usage = func() {
		//p := message.NewPrinter(language.Make(langTag))
		prn.Fprintf(os.Stderr, "Usage: transmission-telegram <-token=TOKEN> <-master=@tuser> [-master=@yuser2] [-url=http://] [-username=user] [-password=pass] [-dir=downloadDirectory]\n\n")
		//fmt.Fprint(os.Stderr, "Usage: transmission-telegram <-token=TOKEN> <-master=@tuser> [-master=@yuser2] [-url=http://] [-username=user] [-password=pass] [-dir=downloadDirectory]\n\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	// if we don't have BotToken passed, check the environment variable "TT_BOTT"
	if BotToken == "" {
		if token := os.Getenv("TT_BOTT"); len(token) > 1 {
			BotToken = token
		}
	}

	// make sure that we have the two madatory arguments: telegram token & master's handler.
	if BotToken == "" ||
		len(Masters) < 1 {
		prn.Fprintf(os.Stderr, "Error: Mandatory argument missing! (-token or -master)\n\n")
		flag.Usage()
		os.Exit(1)
	}

	if DownLoadDir == "" {
		DownLoadDir = RootDir
	}

	// make sure that the handler doesn't contain @
	for i := range Masters {
		Masters[i] = strings.Replace(Masters[i], "@", "", -1)
	}

	DownloadDirs = strings.Split(downsDirs, ",")

	logger.Printf("DownloadDirs count %d", len(DownloadDirs))

	// make sure that the handler doesn't contain ,
	for i := range DownloadDirs {
		DownloadDirs[i] = strings.Replace(DownloadDirs[i], ",", "", -1)
	}

	// if we got a log file, log to it
	if LogFile != "" {
		logf, err := os.OpenFile(LogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal(err)
		}
		logger.SetOutput(logf)
	}

	// if we got a transmission log file, monitor it for torrents completion to notify upon them.
	if TransLogFile != "" {
		go func() {
			ft := tailer.RunFileTailer(TransLogFile, false, nil)

			// [2017-02-22 21:00:00.898] File-Name State changed from "Incomplete" to "Complete" (torrent.c:2218)
			const (
				substring = `"Incomplete" to "Complete"`
				start     = len(`[2017-02-22 21:00:00.898] `)
				end       = len(` State changed from "Incomplete" to "Complete" (torrent.c:2218)`)
			)

			for {
				select {
				case line := <-ft.Lines():
					if strings.Contains(line, substring) {
						// if we don't have a chatID continue
						if chatID == 0 {
							continue
						}

						msg := fmt.Sprintf("Completed: %s", line[start:len(line)-end])
						send(msg, chatID, false)
					}
				case err := <-ft.Errors():
					logger.Printf("[ERROR] tailing transmission log: %s", err)
					return
				}

			}
		}()
	}

	// if the `-username` flag isn't set, look into the environment variable 'TR_AUTH'
	if Username == "" {
		if values := strings.Split(os.Getenv("TR_AUTH"), ":"); len(values) > 1 {
			Username, Password = values[0], values[1]
		}
	}

	// log the flags
	logger.Printf("[INFO] Token=%s\n\t\tMasters=%s\n\t\tURL=%s\n\t\tUSER=%s\n\t\tPASS=%s\n\t\tDownloadDirs=%s\n\t\tRootDir=%s\n\t\tDefaultDir=%s",
		BotToken, Masters, RPCURL, Username, Password, DownloadDirs, RootDir, DefaultDir)
}

// init transmission
func init() {
	prn := message.NewPrinter(language.Make(langTag))
	var err error
	Client, err = transmission.New(RPCURL, Username, Password)
	if err != nil {
		prn.Fprintf(os.Stderr, "[ERROR] Transmission: Make sure you have the right URL, Username and Password\n")
		os.Exit(1)
	}

}

// init telegram
func init() {
	// authorize using the token
	prn := message.NewPrinter(language.Make(langTag))
	var err error
	Bot, err = tgbotapi.NewBotAPI(BotToken)
	if err != nil {
		prn.Fprintf(os.Stderr, "[ERROR] Telegram: %s\n", err)
		os.Exit(1)
	}
	logger.Printf("[INFO] Authorized: %s", Bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	Updates, err = Bot.GetUpdatesChan(u)
	if err != nil {
		prn.Fprintf(os.Stderr, "[ERROR] Telegram: %s\n", err)
		os.Exit(1)
	}
}

func main() {
	//prn := message.NewPrinter(language.Make(langTag))
	for update := range Updates {
		// ignore edited messages
		if update.Message == nil {
			continue
		}

		// ignore non masters
		if !Masters.Contains(update.Message.From.UserName) {
			logger.Printf("[INFO] Ignored a message from: %s", update.Message.From.String())
			continue
		}

		// channel message
		if PrivateOnly && !update.Message.Chat.IsPrivate() {
			logger.Printf("[INFO] Ignored a message from channel/group")
			continue
		}

		// update chatID for complete notification
		if TransLogFile != "" && chatID != update.Message.Chat.ID {
			chatID = update.Message.Chat.ID
		}

		// tokenize the update
		tokens := []string{""}
		if update.Message.Text != "" {
			tokens = strings.Fields(update.Message.Text)

			// preprocess message based on URL schema
			// in case those were added from the mobile via "Share..." option
			// when it is not possible to easily prepend it with "add" command

			if strings.HasPrefix(tokens[0], "magnet") || strings.HasPrefix(tokens[0], "http") {
				tokens = append([]string{"/add"}, tokens...)
			}
		}

		command := strings.ToLower(tokens[0])

		switch command {
		case "list", "/list", "li", "/li":
			go list(update, tokens[1:])

		case "setrootdir", "/setrootdir":
			if len(tokens) == 1 {
				go setdir(update, tokens[1])
			}

		case "getrootdir", "/getrootdir":
			go getdir(update)

		case "head", "/head", "he", "/he":
			go head(update, tokens[1:])

		case "tail", "/tail", "ta", "/ta":
			go tail(update, tokens[1:])

		case "downs", "/downs", "dl", "/dl":
			go downs(update)

		case "seeding", "/seeding", "sd", "/sd":
			go seeding(update)

		case "paused", "/paused", "pa", "/pa":
			go paused(update)

		case "checking", "/checking", "ch", "/ch":
			go checking(update)

		case "active", "/active", "ac", "/ac":
			go active(update)

		case "errors", "/errors", "er", "/er":
			go errors(update)

		case "sort", "/sort", "so", "/so":
			go sort(update, tokens[1:])

		case "trackers", "/trackers", "tr", "/tr":
			go trackers(update)

		case "add", "/add", "ad", "/ad":
			go add(update, tokens[1:2])

		case "search", "/search", "se", "/se":
			go search(update, tokens[1:])

		case "latest", "/latest", "la", "/la":
			go latest(update, tokens[1:])

		case "info", "/info", "in", "/in":
			go info(update, tokens[1:])

		case "stop", "/stop", "sp", "/sp":
			go stop(update, tokens[1:])

		case "start", "/start", "st", "/st":
			go start(update, tokens[1:])

		case "check", "/check", "ck", "/ck":
			go check(update, tokens[1:])

		case "stats", "/stats", "sa", "/sa":
			go stats(update)

		case "speed", "/speed", "ss", "/ss":
			go speed(update)

		case "count", "/count", "co", "/co":
			go count(update)

		case "del", "/del":
			go del(update, tokens[1:])

		case "deldata", "/deldata":
			go deldata(update, tokens[1:])

		case "help", "/help":
			go send(HELP, update.Message.Chat.ID, true)

		case "version", "/version":
			go getVersion(update)

		case "":
			// might be a file received
			go receiveTorrent(update)

		default:
			// no such command, try help
			// go send("No such command, try /help", update.Message.Chat.ID, false)
			//go addMagnetOrTorrent(update)
			go receiveTorrent(update)
		}
	}
}

func setdir(ud tgbotapi.Update, dir string) {
	//DownLoadDir = RootDir + "/" + dir
	RootDir = dir
	send("**setrootdir:** "+RootDir, ud.Message.Chat.ID, true)
}

func setdirasroot(ud tgbotapi.Update) {
	DownLoadDir = RootDir
	send("**setdir:** "+DownLoadDir, ud.Message.Chat.ID, true)
}

func getdir(ud tgbotapi.Update) {
	send(fmt.Sprintf("**getrootdir:**\nRootDir: %s\nDownLoadDir: %s", RootDir, DownLoadDir), ud.Message.Chat.ID, true)
}

// list will form and send a list of all the torrents
// takes an optional argument which is a query to match against trackers
// to list only torrents that has a tracker that matchs.
func list(ud tgbotapi.Update, tokens []string) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**list:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	// if it gets a query, it will list torrents that has trackers that match the query
	if len(tokens) != 0 {
		// (?i) for case insensitivity
		regx, err := regexp.Compile("(?i)" + tokens[0])
		if err != nil {
			send("**list:** "+err.Error(), ud.Message.Chat.ID, true)
			return
		}

		for i := range torrents {
			if regx.MatchString(torrents[i].GetTrackers()) {
				buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
			}
		}
	} else { // if we did not get a query, list all torrents
		for i := range torrents {
			buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
		}
	}

	if buf.Len() == 0 {
		// if we got a tracker query show different message
		if len(tokens) != 0 {
			send(fmt.Sprintf("**list:** No tracker matches: *%s*", tokens[0]), ud.Message.Chat.ID, true)
			return
		}
		send("**list:** no torrents", ud.Message.Chat.ID, true)
		return
	}

	send(buf.String(), ud.Message.Chat.ID, false)
}

// head will list the first 5 or n torrents
func head(ud tgbotapi.Update, tokens []string) {
	var (
		n   = 5 // default to 5
		err error
	)

	if len(tokens) > 0 {
		n, err = strconv.Atoi(tokens[0])
		if err != nil {
			send("**head:** argument must be a number", ud.Message.Chat.ID, true)
			return
		}
	}

	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**head:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	// make sure that we stay in the boundaries
	if n <= 0 || n > len(torrents) {
		n = len(torrents)
	}

	buf := new(bytes.Buffer)
	for i := range torrents[:n] {
		torrentName := mdReplacer.Replace(torrents[i].Name) // escape markdown
		buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
			torrents[i].ID, torrentName, torrents[i].TorrentStatus(), humanize.Bytes(torrents[i].Have()),
			humanize.Bytes(torrents[i].SizeWhenDone), torrents[i].PercentDone*100, humanize.Bytes(torrents[i].RateDownload),
			humanize.Bytes(torrents[i].RateUpload), torrents[i].Ratio()))
	}

	if buf.Len() == 0 {
		send("**head:** no torrents", ud.Message.Chat.ID, true)
		return
	}

	msgID := send(buf.String(), ud.Message.Chat.ID, true)

	if NoLive {
		return
	}

	// keep the info live
	for i := 0; i < duration; i++ {
		time.Sleep(time.Second * interval)
		buf.Reset()

		torrents, err = Client.GetTorrents()
		if err != nil {
			continue // try again if some error heppened
		}

		if len(torrents) < 1 {
			continue
		}

		// make sure that we stay in the boundaries
		if n <= 0 || n > len(torrents) {
			n = len(torrents)
		}

		for _, torrent := range torrents[:n] {
			torrentName := mdReplacer.Replace(torrent.Name) // escape markdown
			buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
				torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()),
				humanize.Bytes(torrent.SizeWhenDone), torrent.PercentDone*100, humanize.Bytes(torrent.RateDownload),
				humanize.Bytes(torrent.RateUpload), torrent.Ratio()))
		}

		// no need to check if it is empty, as if the buffer is empty telegram won't change the message
		editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, buf.String())
		editConf.ParseMode = tgbotapi.ModeMarkdown
		Bot.Send(editConf)
	}

}

// tail lists the last 5 or n torrents
func tail(ud tgbotapi.Update, tokens []string) {
	var (
		n   = 5 // default to 5
		err error
	)

	if len(tokens) > 0 {
		n, err = strconv.Atoi(tokens[0])
		if err != nil {
			send("**tail:** argument must be a number", ud.Message.Chat.ID, true)
			return
		}
	}

	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**tail:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	// make sure that we stay in the boundaries
	if n <= 0 || n > len(torrents) {
		n = len(torrents)
	}

	buf := new(bytes.Buffer)
	for _, torrent := range torrents[len(torrents)-n:] {
		torrentName := mdReplacer.Replace(torrent.Name) // escape markdown
		buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
			torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()),
			humanize.Bytes(torrent.SizeWhenDone), torrent.PercentDone*100, humanize.Bytes(torrent.RateDownload),
			humanize.Bytes(torrent.RateUpload), torrent.Ratio()))
	}

	if buf.Len() == 0 {
		send("**tail:** no torrents", ud.Message.Chat.ID, true)
		return
	}

	msgID := send(buf.String(), ud.Message.Chat.ID, true)

	if NoLive {
		return
	}

	// keep the info live
	for i := 0; i < duration; i++ {
		time.Sleep(time.Second * interval)
		buf.Reset()

		torrents, err = Client.GetTorrents()
		if err != nil {
			continue // try again if some error heppened
		}

		if len(torrents) < 1 {
			continue
		}

		// make sure that we stay in the boundaries
		if n <= 0 || n > len(torrents) {
			n = len(torrents)
		}

		for _, torrent := range torrents[len(torrents)-n:] {
			torrentName := mdReplacer.Replace(torrent.Name) // escape markdown
			buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
				torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()),
				humanize.Bytes(torrent.SizeWhenDone), torrent.PercentDone*100, humanize.Bytes(torrent.RateDownload),
				humanize.Bytes(torrent.RateUpload), torrent.Ratio()))
		}

		// no need to check if it is empty, as if the buffer is empty telegram won't change the message
		editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, buf.String())
		editConf.ParseMode = tgbotapi.ModeMarkdown
		Bot.Send(editConf)
	}

}

// downs will send the names of torrents with status 'Downloading' or in queue to
func downs(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**downs:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		// Downloading or in queue to download
		if torrents[i].Status == transmission.StatusDownloading ||
			torrents[i].Status == transmission.StatusDownloadPending {
			buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
		}
	}

	if buf.Len() == 0 {
		send("No downloads", ud.Message.Chat.ID, false)
		return
	}
	send(buf.String(), ud.Message.Chat.ID, false)
}

// seeding will send the names of the torrents with the status 'Seeding' or in the queue to
func seeding(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**seeding:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if torrents[i].Status == transmission.StatusSeeding ||
			torrents[i].Status == transmission.StatusSeedPending {
			buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
		}
	}

	if buf.Len() == 0 {
		send("No torrents seeding", ud.Message.Chat.ID, false)
		return
	}

	send(buf.String(), ud.Message.Chat.ID, false)

}

// paused will send the names of the torrents with status 'Paused'
func paused(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**paused:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if torrents[i].Status == transmission.StatusStopped {
			buf.WriteString(fmt.Sprintf("<%d> %s\n%s (%.1f%%) DL: %s UL: %s  R: %s\n\n",
				torrents[i].ID, torrents[i].Name, torrents[i].TorrentStatus(),
				torrents[i].PercentDone*100, humanize.Bytes(torrents[i].DownloadedEver),
				humanize.Bytes(torrents[i].UploadedEver), torrents[i].Ratio()))
		}
	}

	if buf.Len() == 0 {
		send("No paused torrents", ud.Message.Chat.ID, false)
		return
	}

	send(buf.String(), ud.Message.Chat.ID, false)
}

// checking will send the names of torrents with the status 'verifying' or in the queue to
func checking(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**checking:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if torrents[i].Status == transmission.StatusChecking ||
			torrents[i].Status == transmission.StatusCheckPending {
			buf.WriteString(fmt.Sprintf("<%d> %s\n%s (%.1f%%)\n\n",
				torrents[i].ID, torrents[i].Name, torrents[i].TorrentStatus(),
				torrents[i].PercentDone*100))

		}
	}

	if buf.Len() == 0 {
		send("No torrents verifying", ud.Message.Chat.ID, false)
		return
	}

	send(buf.String(), ud.Message.Chat.ID, false)
}

// active will send torrents that are actively downloading or uploading
func active(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**active:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if torrents[i].RateDownload > 0 ||
			torrents[i].RateUpload > 0 {
			// escape markdown
			torrentName := mdReplacer.Replace(torrents[i].Name)
			buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
				torrents[i].ID, torrentName, torrents[i].TorrentStatus(), humanize.Bytes(torrents[i].Have()),
				humanize.Bytes(torrents[i].SizeWhenDone), torrents[i].PercentDone*100, humanize.Bytes(torrents[i].RateDownload),
				humanize.Bytes(torrents[i].RateUpload), torrents[i].Ratio()))
		}
	}
	if buf.Len() == 0 {
		send("No active torrents", ud.Message.Chat.ID, false)
		return
	}

	msgID := send(buf.String(), ud.Message.Chat.ID, true)

	if NoLive {
		return
	}

	// keep the active list live for 'duration * interval'
	for i := 0; i < duration; i++ {
		time.Sleep(time.Second * interval)
		// reset the buffer to reuse it
		buf.Reset()

		// update torrents
		torrents, err = Client.GetTorrents()
		if err != nil {
			continue // if there was error getting torrents, skip to the next iteration
		}

		// do the same loop again
		for i := range torrents {
			if torrents[i].RateDownload > 0 ||
				torrents[i].RateUpload > 0 {
				torrentName := mdReplacer.Replace(torrents[i].Name) // replace markdown chars
				buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\n\n",
					torrents[i].ID, torrentName, torrents[i].TorrentStatus(), humanize.Bytes(torrents[i].Have()),
					humanize.Bytes(torrents[i].SizeWhenDone), torrents[i].PercentDone*100, humanize.Bytes(torrents[i].RateDownload),
					humanize.Bytes(torrents[i].RateUpload), torrents[i].Ratio()))
			}
		}

		// no need to check if it is empty, as if the buffer is empty telegram won't change the message
		editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, buf.String())
		editConf.ParseMode = tgbotapi.ModeMarkdown
		Bot.Send(editConf)
	}
	// sleep one more time before putting the dashes
	time.Sleep(time.Second * interval)

	// replace the speed with dashes to indicate that we are done being live
	buf.Reset()
	for i := range torrents {
		if torrents[i].RateDownload > 0 ||
			torrents[i].RateUpload > 0 {
			// escape markdown
			torrentName := mdReplacer.Replace(torrents[i].Name)
			buf.WriteString(fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *-*  ↑ *-* R: *%s*\n\n",
				torrents[i].ID, torrentName, torrents[i].TorrentStatus(), humanize.Bytes(torrents[i].Have()),
				humanize.Bytes(torrents[i].SizeWhenDone), torrents[i].PercentDone*100, torrents[i].Ratio()))
		}
	}

	editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, buf.String())
	editConf.ParseMode = tgbotapi.ModeMarkdown
	Bot.Send(editConf)

}

// errors will send torrents with errors
func errors(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**errors:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if torrents[i].Error != 0 {
			buf.WriteString(fmt.Sprintf("<%d> %s\n%s\n",
				torrents[i].ID, torrents[i].Name, torrents[i].ErrorString))
		}
	}
	if buf.Len() == 0 {
		send("No errors", ud.Message.Chat.ID, false)
		return
	}
	send(buf.String(), ud.Message.Chat.ID, false)
}

// sort changes torrents sorting
func sort(ud tgbotapi.Update, tokens []string) {
	if len(tokens) == 0 {
		send(`**sort** takes one of:
			(*id, name, age, size, progress, downspeed, upspeed, download, upload, ratio*)
			optionally start with (**rev**) for reversed order
			e.g. "*sort rev size*" to get biggest torrents first.`, ud.Message.Chat.ID, true)
		return
	}

	var reversed bool
	if strings.ToLower(tokens[0]) == "rev" {
		reversed = true
		tokens = tokens[1:]
	}

	switch strings.ToLower(tokens[0]) {
	case "id":
		if reversed {
			Client.SetSort(transmission.SortRevID)
			break
		}
		Client.SetSort(transmission.SortID)
	case "name":
		if reversed {
			Client.SetSort(transmission.SortRevName)
			break
		}
		Client.SetSort(transmission.SortName)
	case "age":
		if reversed {
			Client.SetSort(transmission.SortRevAge)
			break
		}
		Client.SetSort(transmission.SortAge)
	case "size":
		if reversed {
			Client.SetSort(transmission.SortRevSize)
			break
		}
		Client.SetSort(transmission.SortSize)
	case "progress":
		if reversed {
			Client.SetSort(transmission.SortRevProgress)
			break
		}
		Client.SetSort(transmission.SortProgress)
	case "downspeed":
		if reversed {
			Client.SetSort(transmission.SortRevDownSpeed)
			break
		}
		Client.SetSort(transmission.SortDownSpeed)
	case "upspeed":
		if reversed {
			Client.SetSort(transmission.SortRevUpSpeed)
			break
		}
		Client.SetSort(transmission.SortUpSpeed)
	case "download":
		if reversed {
			Client.SetSort(transmission.SortRevDownloaded)
			break
		}
		Client.SetSort(transmission.SortDownloaded)
	case "upload":
		if reversed {
			Client.SetSort(transmission.SortRevUploaded)
			break
		}
		Client.SetSort(transmission.SortUploaded)
	case "ratio":
		if reversed {
			Client.SetSort(transmission.SortRevRatio)
			break
		}
		Client.SetSort(transmission.SortRatio)
	default:
		send("unkown sorting method", ud.Message.Chat.ID, false)
		return
	}

	if reversed {
		send("**sort:** reversed "+tokens[0], ud.Message.Chat.ID, true)
		return
	}
	send("**sort:** "+tokens[0], ud.Message.Chat.ID, true)
}

var trackerRegex = regexp.MustCompile(`[https?|udp]://([^:/]*)`)

// trackers will send a list of trackers and how many torrents each one has
func trackers(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**trackers:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	trackers := make(map[string]int)

	for i := range torrents {
		for _, tracker := range torrents[i].Trackers {
			sm := trackerRegex.FindSubmatch([]byte(tracker.Announce))
			if len(sm) > 1 {
				currentTracker := string(sm[1])
				n, ok := trackers[currentTracker]
				if !ok {
					trackers[currentTracker] = 1
					continue
				}
				trackers[currentTracker] = n + 1
			}
		}
	}

	buf := new(bytes.Buffer)
	for k, v := range trackers {
		buf.WriteString(fmt.Sprintf("%d - %s\n", v, k))
	}

	if buf.Len() == 0 {
		send("No trackers!", ud.Message.Chat.ID, false)
		return
	}
	send(buf.String(), ud.Message.Chat.ID, false)
}

func testLink(url string) bool {
	if strings.HasPrefix(url, "magnet") || strings.HasPrefix(url, "http") {
		return true
	} else {
		return false
	}
}

// add takes an URL to a .torrent file to add it to transmission
func add(ud tgbotapi.Update, tokens []string) {
	if len(tokens) == 0 {
		send("**add:** needs at least one URL", ud.Message.Chat.ID, true)
		return
	}

	//tokens
	//Первый элемент - сылка на торрент
	//Второй элемент - имя подпапки из downloadDirs
	//сравнение идет без учета регистра
	//если папка не найдена - идет скачивание в дефолтную папку
	url := tokens[0]
	folder := tokens[1]
	//logger.Printf("get folder %s", folder)
	if testLink(url) {
		//DownloadDirs
		//nums := []int{8, 0}
		//nums = append(nums, 8)
		for i := range DownloadDirs {
			if strings.ToLower(tokens[1]) == strings.ToLower(DownloadDirs[i]) {
				folder = DownloadDirs[i]
				//logger.Printf("set folder is %s", DownloadDirs[i])
				break
			} else {
				folder = ""
			}
		}
		if folder == "" {
			folder = DefaultDir
		}
		var cmd *transmission.Command
		cmd = transmission.NewAddCmdByURLWithDir(url, RootDir+"/"+folder)

		torrent, err := Client.ExecuteAddCommand(cmd)
		if err != nil && err != transmission.ErrTorrentDuplicate {
			send("**add:** "+err.Error(), ud.Message.Chat.ID, true)
			return
		}

		// check if torrent.Name is empty, then an error happened
		if torrent.Name == "" {
			send("**add:** error adding "+url, ud.Message.Chat.ID, true)
			return
		}
		if err == nil {
			send(fmt.Sprintf("**Added:** <%d> %s to folder %s", torrent.ID, torrent.Name, folder), ud.Message.Chat.ID, true)
		} else {
			send(fmt.Sprintf("**Duplicated:** <%d> %s", torrent.ID, torrent.Name), ud.Message.Chat.ID, true)
		}
	}
}

// receiveTorrent gets an update that potentially has a .torrent file to add
func receiveTorrent(ud tgbotapi.Update) {
	if ud.Message.Document == nil {
		return // has no document
	}
	//ud.Message.Text
	// get the file ID and make the config
	fconfig := tgbotapi.FileConfig{
		FileID: ud.Message.Document.FileID,
	}
	file, err := Bot.GetFile(fconfig)
	if err != nil {
		send("**receiver:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	//logger.Printf("received %s", ud.Message.Caption)

	// add by file URL
	add(ud, []string{file.Link(BotToken), ud.Message.Caption})
}

// add torrent files in entities or explict magnet link
func addMagnetOrTorrent(ud tgbotapi.Update) {
	var tokens []string
	checkMagnetLink := false
	if ud.Message.Entities != nil {
		for idx := 0; idx < len(*ud.Message.Entities); idx++ {
			entity := (*ud.Message.Entities)[idx]
			if entity.URL != "" && strings.ToLower(entity.URL[len(entity.URL)-8:]) == ".torrent" {
				tokens = append(tokens, entity.URL)
			}
		}
	}

	if len(tokens) == 0 && strings.Contains(ud.Message.Text, "magnet:") {
		r, _ := regexp.Compile("magnet:\\?xt=urn:[a-zA-Z0-9]+:[a-zA-Z0-9]{32,40}(&dn=.+&tr=.+)?")
		tokens = r.FindAllString(ud.Message.Text, -1)
		checkMagnetLink = true
	}
	if len(tokens) > 0 {
		add(ud, tokens)
	} else {
		s := ""
		if ud.Message.Entities != nil {
			s = "I don't find torrent files"
		} else if checkMagnetLink {
			s = fmt.Sprintf("I don't find magnetLink from __%q__", ud.Message.Text)
		}
		if s != "" {
			send(s, ud.Message.Chat.ID, true)
		}
	}
}

// search takes a query and returns torrents with match
func search(ud tgbotapi.Update, tokens []string) {
	// make sure that we got a query
	if len(tokens) == 0 {
		send("**search:** needs an argument", ud.Message.Chat.ID, true)
		return
	}

	query := strings.Join(tokens, " ")
	// "(?i)" for case insensitivity
	regx, err := regexp.Compile("(?i)" + query)
	if err != nil {
		send("**search:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**search:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	buf := new(bytes.Buffer)
	for i := range torrents {
		if regx.MatchString(torrents[i].Name) {
			buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
		}
	}
	if buf.Len() == 0 {
		send("No matches!", ud.Message.Chat.ID, false)
		return
	}
	send(buf.String(), ud.Message.Chat.ID, false)
}

// latest takes n and returns the latest n torrents
func latest(ud tgbotapi.Update, tokens []string) {
	var (
		n   = 5 // default to 5
		err error
	)

	if len(tokens) > 0 {
		n, err = strconv.Atoi(tokens[0])
		if err != nil {
			send("**latest:** argument must be a number", ud.Message.Chat.ID, true)
			return
		}
	}

	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**latest:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	// make sure that we stay in the boundaries
	if n <= 0 || n > len(torrents) {
		n = len(torrents)
	}

	// sort by age, and set reverse to true to get the latest first
	torrents.SortAge(true)

	buf := new(bytes.Buffer)
	for i := range torrents[:n] {
		buf.WriteString(fmt.Sprintf("<%d> %s\n", torrents[i].ID, torrents[i].Name))
	}
	if buf.Len() == 0 {
		send("**latest:** No torrents", ud.Message.Chat.ID, true)
		return
	}
	send(buf.String(), ud.Message.Chat.ID, false)
}

// info takes an id of a torrent and returns some info about it
func info(ud tgbotapi.Update, tokens []string) {
	if len(tokens) == 0 {
		send("**info:** needs a torrent ID number", ud.Message.Chat.ID, true)
		return
	}

	for _, id := range tokens {
		torrentID, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**info:** %s is not a number", id), ud.Message.Chat.ID, true)
			continue
		}

		// get the torrent
		torrent, err := Client.GetTorrent(torrentID)
		if err != nil {
			send(fmt.Sprintf("**info:** Can't find a torrent with an ID of %d", torrentID), ud.Message.Chat.ID, true)
			continue
		}

		// get the trackers using 'trackerRegex'
		var trackers string
		for _, tracker := range torrent.Trackers {
			sm := trackerRegex.FindSubmatch([]byte(tracker.Announce))
			if len(sm) > 1 {
				trackers += string(sm[1]) + " "
			}
		}

		// format the info
		torrentName := mdReplacer.Replace(torrent.Name) // escape markdown
		info := fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\nDL: *%s* UP: *%s*\nAdded: *%s*, ETA: *%s*\nTrackers: `%s`",
			torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()), humanize.Bytes(torrent.SizeWhenDone),
			torrent.PercentDone*100, humanize.Bytes(torrent.RateDownload), humanize.Bytes(torrent.RateUpload), torrent.Ratio(),
			humanize.Bytes(torrent.DownloadedEver), humanize.Bytes(torrent.UploadedEver), time.Unix(torrent.AddedDate, 0).Format(time.Stamp),
			torrent.ETA(), trackers)

		// send it
		msgID := send(info, ud.Message.Chat.ID, true)

		if NoLive {
			return
		}

		// this go-routine will make the info live for 'duration * interval'
		go func(torrentID, msgID int) {
			for i := 0; i < duration; i++ {
				time.Sleep(time.Second * interval)
				torrent, err = Client.GetTorrent(torrentID)
				if err != nil {
					continue // skip this iteration if there's an error retrieving the torrent's info
				}

				torrentName := mdReplacer.Replace(torrent.Name)
				info := fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *%s*  ↑ *%s* R: *%s*\nDL: *%s* UP: *%s*\nAdded: *%s*, ETA: *%s*\nTrackers: `%s`",
					torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()), humanize.Bytes(torrent.SizeWhenDone),
					torrent.PercentDone*100, humanize.Bytes(torrent.RateDownload), humanize.Bytes(torrent.RateUpload), torrent.Ratio(),
					humanize.Bytes(torrent.DownloadedEver), humanize.Bytes(torrent.UploadedEver), time.Unix(torrent.AddedDate, 0).Format(time.Stamp),
					torrent.ETA(), trackers)

				// update the message
				editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, info)
				editConf.ParseMode = tgbotapi.ModeMarkdown
				Bot.Send(editConf)

			}
			// sleep one more time before the dashes
			time.Sleep(time.Second * interval)

			// at the end write dashes to indicate that we are done being live.
			torrentName := mdReplacer.Replace(torrent.Name)
			info := fmt.Sprintf("`<%d>` *%s*\n%s *%s* of *%s* (*%.1f%%*) ↓ *- B*  ↑ *- B* R: *%s*\nDL: *%s* UP: *%s*\nAdded: *%s*, ETA: *-*\nTrackers: `%s`",
				torrent.ID, torrentName, torrent.TorrentStatus(), humanize.Bytes(torrent.Have()), humanize.Bytes(torrent.SizeWhenDone),
				torrent.PercentDone*100, torrent.Ratio(), humanize.Bytes(torrent.DownloadedEver), humanize.Bytes(torrent.UploadedEver),
				time.Unix(torrent.AddedDate, 0).Format(time.Stamp), trackers)

			editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, info)
			editConf.ParseMode = tgbotapi.ModeMarkdown
			Bot.Send(editConf)
		}(torrentID, msgID)
	}
}

// stop takes id[s] of torrent[s] or 'all' to stop them
func stop(ud tgbotapi.Update, tokens []string) {
	// make sure that we got at least one argument
	if len(tokens) == 0 {
		send("**stop:** needs an argument", ud.Message.Chat.ID, true)
		return
	}

	// if the first argument is 'all' then stop all torrents
	if tokens[0] == "all" {
		if err := Client.StopAll(); err != nil {
			send("**stop:** error occurred while stopping some torrents", ud.Message.Chat.ID, true)
			return
		}
		send("Stopped all torrents", ud.Message.Chat.ID, false)
		return
	}

	for _, id := range tokens {
		num, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**stop:** %s is not a number", id), ud.Message.Chat.ID, true)
			continue
		}
		status, err := Client.StopTorrent(num)
		if err != nil {
			send("**stop:** "+err.Error(), ud.Message.Chat.ID, true)
			continue
		}

		torrent, err := Client.GetTorrent(num)
		if err != nil {
			send(fmt.Sprintf("[fail] **stop:** No torrent with an ID of %d", num), ud.Message.Chat.ID, true)
			return
		}
		send(fmt.Sprintf("[%s] **stop:** %s", status, torrent.Name), ud.Message.Chat.ID, true)
	}
}

// start takes id[s] of torrent[s] or 'all' to start them
func start(ud tgbotapi.Update, tokens []string) {
	// make sure that we got at least one argument
	if len(tokens) == 0 {
		send("**start:** needs an argument", ud.Message.Chat.ID, true)
		return
	}

	// if the first argument is 'all' then start all torrents
	if tokens[0] == "all" {
		if err := Client.StartAll(); err != nil {
			send("**start:** error occurred while starting some torrents", ud.Message.Chat.ID, true)
			return
		}
		send("Started all torrents", ud.Message.Chat.ID, false)
		return

	}

	for _, id := range tokens {
		num, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**start:** %s is not a number", id), ud.Message.Chat.ID, true)
			continue
		}
		status, err := Client.StartTorrent(num)
		if err != nil {
			send("**start:** "+err.Error(), ud.Message.Chat.ID, true)
			continue
		}

		torrent, err := Client.GetTorrent(num)
		if err != nil {
			send(fmt.Sprintf("[fail] **start:** No torrent with an ID of %d", num), ud.Message.Chat.ID, true)
			return
		}
		send(fmt.Sprintf("[%s] **start:** %s", status, torrent.Name), ud.Message.Chat.ID, true)
	}
}

// check takes id[s] of torrent[s] or 'all' to verify them
func check(ud tgbotapi.Update, tokens []string) {
	// make sure that we got at least one argument
	if len(tokens) == 0 {
		send("**check:** needs an argument", ud.Message.Chat.ID, true)
		return
	}

	// if the first argument is 'all' then start all torrents
	if tokens[0] == "all" {
		if err := Client.VerifyAll(); err != nil {
			send("**check:** error occurred while verifying some torrents", ud.Message.Chat.ID, true)
			return
		}
		send("Verifying all torrents", ud.Message.Chat.ID, false)
		return

	}

	for _, id := range tokens {
		num, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**check:** %s is not a number", id), ud.Message.Chat.ID, true)
			continue
		}
		status, err := Client.VerifyTorrent(num)
		if err != nil {
			send("**check:** "+err.Error(), ud.Message.Chat.ID, true)
			continue
		}

		torrent, err := Client.GetTorrent(num)
		if err != nil {
			send(fmt.Sprintf("[fail] **check:** No torrent with an ID of %d", num), ud.Message.Chat.ID, true)
			return
		}
		send(fmt.Sprintf("[%s] **check:** %s", status, torrent.Name), ud.Message.Chat.ID, true)
	}

}

// stats echo back transmission stats
func stats(ud tgbotapi.Update) {
	stats, err := Client.GetStats()
	if err != nil {
		send("**stats:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	msg := fmt.Sprintf(
		`
		Total: *%d*
		Active: *%d*
		Paused: *%d*

		_Current Stats_
		Downloaded: *%s*
		Uploaded: *%s*
		Running time: *%s*

		_Accumulative Stats_
		Sessions: *%d*
		Downloaded: *%s*
		Uploaded: *%s*
		Total Running time: *%s*
		`,

		stats.TorrentCount,
		stats.ActiveTorrentCount,
		stats.PausedTorrentCount,
		humanize.Bytes(stats.CurrentStats.DownloadedBytes),
		humanize.Bytes(stats.CurrentStats.UploadedBytes),
		stats.CurrentActiveTime(),
		stats.CumulativeStats.SessionCount,
		humanize.Bytes(stats.CumulativeStats.DownloadedBytes),
		humanize.Bytes(stats.CumulativeStats.UploadedBytes),
		stats.CumulativeActiveTime(),
	)

	send(msg, ud.Message.Chat.ID, true)
}

// speed will echo back the current download and upload speeds
func speed(ud tgbotapi.Update) {
	stats, err := Client.GetStats()
	if err != nil {
		send("**speed:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	msg := fmt.Sprintf("↓ %s  ↑ %s", humanize.Bytes(stats.DownloadSpeed), humanize.Bytes(stats.UploadSpeed))

	msgID := send(msg, ud.Message.Chat.ID, false)

	if NoLive {
		return
	}

	for i := 0; i < duration; i++ {
		time.Sleep(time.Second * interval)
		stats, err = Client.GetStats()
		if err != nil {
			continue
		}

		msg = fmt.Sprintf("↓ %s  ↑ %s", humanize.Bytes(stats.DownloadSpeed), humanize.Bytes(stats.UploadSpeed))

		editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, msg)
		Bot.Send(editConf)
		time.Sleep(time.Second * interval)
	}
	// sleep one more time before switching to dashes
	time.Sleep(time.Second * interval)

	// show dashes to indicate that we are done updating.
	editConf := tgbotapi.NewEditMessageText(ud.Message.Chat.ID, msgID, "↓ - B  ↑ - B")
	Bot.Send(editConf)
}

// count returns current torrents count per status
func count(ud tgbotapi.Update) {
	torrents, err := Client.GetTorrents()
	if err != nil {
		send("**count:** "+err.Error(), ud.Message.Chat.ID, true)
		return
	}

	var downloading, seeding, stopped, checking, downloadingQ, seedingQ, checkingQ int

	for i := range torrents {
		switch torrents[i].Status {
		case transmission.StatusDownloading:
			downloading++
		case transmission.StatusSeeding:
			seeding++
		case transmission.StatusStopped:
			stopped++
		case transmission.StatusChecking:
			checking++
		case transmission.StatusDownloadPending:
			downloadingQ++
		case transmission.StatusSeedPending:
			seedingQ++
		case transmission.StatusCheckPending:
			checkingQ++
		}
	}

	msg := fmt.Sprintf("Downloading: %d\nSeeding: %d\nPaused: %d\nVerifying: %d\n\n- Waiting to -\nDownload: %d\nSeed: %d\nVerify: %d\n\nTotal: %d",
		downloading, seeding, stopped, checking, downloadingQ, seedingQ, checkingQ, len(torrents))

	send(msg, ud.Message.Chat.ID, false)

}

// del takes an id or more, and delete the corresponding torrent/s
func del(ud tgbotapi.Update, tokens []string) {
	// make sure that we got an argument
	if len(tokens) == 0 {
		send("**del:** needs an ID", ud.Message.Chat.ID, true)
		return
	}

	// loop over tokens to read each potential id
	for _, id := range tokens {
		num, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**del:** %s is not an ID", id), ud.Message.Chat.ID, true)
			return
		}

		name, err := Client.DeleteTorrent(num, false)
		if err != nil {
			send("**del:** "+err.Error(), ud.Message.Chat.ID, true)
			return
		}

		send("**Deleted:** "+name, ud.Message.Chat.ID, true)
	}
}

// deldata takes an id or more, and delete the corresponding torrent/s with their data
func deldata(ud tgbotapi.Update, tokens []string) {
	// make sure that we got an argument
	if len(tokens) == 0 {
		send("**deldata:** needs an ID", ud.Message.Chat.ID, true)
		return
	}
	// loop over tokens to read each potential id
	for _, id := range tokens {
		num, err := strconv.Atoi(id)
		if err != nil {
			send(fmt.Sprintf("**deldata:** %s is not an ID", id), ud.Message.Chat.ID, true)
			return
		}

		name, err := Client.DeleteTorrent(num, true)
		if err != nil {
			send("**deldata:** "+err.Error(), ud.Message.Chat.ID, true)
			return
		}

		send("Deleted with data: "+name, ud.Message.Chat.ID, false)
	}
}

// getVersion sends transmission version + transmission-telegram version
func getVersion(ud tgbotapi.Update) {
	send(fmt.Sprintf("Transmission *%s*\nTransmission-telegram *%s*", Client.Version(), VERSION), ud.Message.Chat.ID, true)
}

// send takes a chat id and a message to send, returns the message id of the send message
func send(text string, chatID int64, markdown bool) int {
	// set typing action
	action := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	Bot.Send(action)

	// check the rune count, telegram is limited to 4096 chars per message;
	// so if our message is > 4096, split it in chunks the send them.
	msgRuneCount := utf8.RuneCountInString(text)
LenCheck:
	stop := 4095
	if msgRuneCount > 4096 {
		for text[stop] != 10 { // '\n'
			stop--
		}
		msg := tgbotapi.NewMessage(chatID, text[:stop])
		msg.DisableWebPagePreview = true
		if markdown {
			msg.ParseMode = tgbotapi.ModeMarkdown
		}

		// send current chunk
		if _, err := Bot.Send(msg); err != nil {
			logger.Printf("[ERROR] Send: %s", err)
		}
		// move to the next chunk
		text = text[stop:]
		msgRuneCount = utf8.RuneCountInString(text)
		goto LenCheck
	}

	// if msgRuneCount < 4096, send it normally
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	if markdown {
		msg.ParseMode = tgbotapi.ModeMarkdown
	}

	resp, err := Bot.Send(msg)
	if err != nil {
		logger.Printf("[ERROR] Send: %s", err)
	}

	return resp.MessageID
}
