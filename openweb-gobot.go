// Steve Phillips / elimisteve
// 2011.03.25

package main

import (
	"encoding/json"
	"fmt"
	"github.com/elimisteve/fun"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"./redmine"
	"regexp"
	"strings"
	"time"
)

const (
	IRC_SERVER         = "irc.freenode.net:6667"

	// BOT_NICK           = "openweb_gobottest"
	// IRC_CHANNEL        = "#openwebtest"
	// THIS_SERVER_NAME   = "openweb-core"
	// VERBOSE            = true

	BOT_NICK           = "openweb_gobot"
	IRC_CHANNEL        = "#openwebengineering"
	THIS_SERVER_NAME   = "openweb-core"
	VERBOSE            = true
	// BOT_NICK           = "openweb_gobot2"
	// IRC_CHANNEL        = "#prototypemagic"
	// THIS_SERVER_NAME   = "openweb-micro2"
	// VERBOSE            = false
	// BOT_NICK           = "openweb_gobot3"
	// IRC_CHANNEL        = "#prototypemagic"
	// THIS_SERVER_NAME   = "the Linode box"
	// VERBOSE            = false

	REDMINE_URL        = ""
	REPO_BASE_PATH     = "/home/ubuntu/openwebengineering_code/"
	OWNER_NICK         = "elimisteve"

	PRIVMSG_PRE        = "PRIVMSG "
	PRIVMSG_POST       = " :"
	IRC_CHAN_MSG_PRE   = PRIVMSG_PRE + IRC_CHANNEL + PRIVMSG_POST
	REPO_INDEX_FILE    = ".index"
	GIT_PORT           = "6666"
	WEBHOOK_PORT       = "7777"
	LOCAL_GITHUB_REPOS = "/home/ubuntu/github_repos/" // NOT USED right now
	REVENGE_MSG        = "I never forgive. I never forget."
)

var NICKS_TO_NOTIFY = []string{"elimisteve", "elimisteve1", "elimisteve11",
                               "elimisteve12"}

type GitCommit struct {
	Author string
	Email string
	Repo string
	RepoOwner string
	Message string
	Date string
	Hash string
}

func checkError(where string, err error) {
	if err != nil {
		log.Fatalf(where + ": " + err.Error())
	}
}

// Only one connection allowed (subject to change)
var conn net.Conn

// Anything passed to this channel is echoed into IRC_CHANNEL
var irc = make(chan string)

var revenge = []string{}

func main() {
	// If the constants are valid, this program cannot crash. Period.
	defer func() {
		if err := recover(); err != nil {
			msg := fmt.Sprintf("Recovered from nasty error in main: %v\n", err)
			// ircMsg(msg)
			fmt.Print(msg)
		}
	}()

	// Connect to IRC
	conn = ircSetup()
	defer conn.Close()

	// Listen for repo names on port GIT_PORT, then echo info
	// from latest commit into IRC_CHANNEL. Currently triggered by
	// post-receive git hooks.
	go gitListener()

	// Listen for (WebHook-powered) JSON POSTs from GitHub to port
	// WEBHOOK_PORT
	go webhookListener()

	// Anything passed to the `irc` channel (get it?) is echoed into
	// IRC_CHANNEL
	go func() {
		for { ircMsg(<-irc) }
	}()

	//
	// Main loop
	//
	read_buf := make([]byte, 512)
	for {
		// n bytes read
		n, err := conn.Read(read_buf)
		checkError("conn.Read", err)
		data := string(read_buf[:n])
		data = strings.TrimRight(data, " \t\r\n") // Remove trailing whitespace
		fmt.Printf("%v\n", data)
		//
		// Respond to PING
		//
		if strings.HasPrefix(data, "PING") {
			rawIrcMsg("PONG " + data)
		}
		//
		// Parse nick, msg
		//

		// Avoids ~global var risk by resetting these to "" each loop
		var msg, nick string = "", ""

		// Parse nick when safe to do so
		// TODO: Look at IRC spec; not sure this is guaranteed to work
		if fun.ContainsAnyStrings(data, "PRIVMSG", "MODE", "JOIN", "KICK") {
			// structure of `data` == :nick!host PRIVMSG #channel :msg

			// nick == everything after first char, before first !
			nick = strings.SplitN(data[1:], "!", 2)[0]
			fmt.Printf("Nick: '%v'\n", nick)
		}
		// Parse msg when safe to do so
		// TODO: Make this much more precise
		if fun.ContainsAnyStrings(data, "PRIVMSG", "KICK") {
			// msg == everything after second :
			msg = strings.SplitN(data, ":", 3)[2]
			fmt.Printf("Message: '%v'\n", msg)
		}
		// Thank user when given OP... then seek revenge
		// TODO: Use regex to parse `data`
		if strings.Contains(data, "MODE " + IRC_CHANNEL + " +o " + BOT_NICK) {
			irc <- nick + ": thanks :-)"
			for _, user := range revenge {
				rawIrcMsg("KICK " + IRC_CHANNEL + " " + user + " :" + REVENGE_MSG)
			}
			revenge = []string{}
		}
		// Seek revenge on those who remove bot's OP
		// TODO: Use regex to parse `data`
		if strings.Contains(data, "MODE " + IRC_CHANNEL + " -o " + BOT_NICK) {
			irc <- ":-("
			revenge = append(revenge, nick)
		}
		//
		// Re-join if kicked
		//
		if fun.ContainsAllStrings(data, "KICK", BOT_NICK) {
			rawIrcMsg("JOIN " + IRC_CHANNEL)
			revenge = append(revenge, nick)
		}
		// We don't need more than one bot posting Redmine tickets...
		// TODO: Create p2p bot federation (no master//elected master)
		if THIS_SERVER_NAME == "openweb-core" {
			if fun.HasAnyPrefixes(msg, "!ticket ", "!new ", "!bug ") {
				go func() {
					ticket := redmine.ParseTicket(msg)
					if ticket == nil {
						irc <- "Usage: '![new|bug] project_name New Ticket " +
							"Subject; New ticket description'"
						return
					}
					resp, err := redmine.CreateTicket(ticket)
					if err != nil {
						irc <- "Ticket creation failed: " + err.Error()
					}
					if resp.StatusCode == 404 {
						irc <- fmt.Sprintf("Project '%v' not found",
							ticket.Project)
					} else if resp.StatusCode == 201 {
						irc <- "Ticket created: " + resp.Header["Location"][0]
					} else {
						irc <- "Something bad happened..."
						log.Printf("resp == %+v\n", resp)
						body, err := ioutil.ReadAll(resp.Body)
						if err != nil {
							irc <- "Error reading response, even!"
							return
						}
						resp.Body.Close()
						log.Printf("Server response:\n%s\n", body)
					}
					return
				}()
			}
		}
		//
		// ADD YOUR CODE (or function calls) HERE
		//
	}
}

func ircSetup() net.Conn {
	var err error
	// Avoid the temptation... `conn, err := ...` silently shadows the
	// global `conn` variable!
	conn, err = net.Dial("tcp", IRC_SERVER)
	checkError("net.Dial", err)

	rawIrcMsg("NICK " + BOT_NICK)
	rawIrcMsg("USER " + strings.Repeat(BOT_NICK+" ", 4))
	rawIrcMsg("JOIN " + IRC_CHANNEL)
	return conn
}

// rawIrcMsg takes a string and writes it to the global TCP connection
// to the IRC server _verbatim_
func rawIrcMsg(str string) {
	conn.Write([]uint8(str + "\n"))
}

// ircMsg is a helper function that wraps rawIrcMsg, prefacing each
// message with IRC_CHAN_MSG_PRE (usually `PRIVMSG $IRC_CHANNEL `)
func ircMsg(msg string) {
	rawIrcMsg(IRC_CHAN_MSG_PRE + msg)
}

// privMsg is a helper function that wraps rawIrcMsg, prefacing each
// message with IRC_CHAN_MSG_PRE (usually `PRIVMSG $IRC_CHANNEL `)
func privMsg(nickOrChannel, msg string) {
	rawIrcMsg(PRIVMSG_PRE + nickOrChannel + PRIVMSG_POST + msg)
}

func privMsgOwner(msg string) {
	privMsg(OWNER_NICK, msg)
}

func repoToNonMergeCommit(fullRepoPath, repoName string) GitCommit {
	// defer func(where string) {
	// 	if err := recover(); err != nil {
	// 		msg := fmt.Sprintf("Recovered from error in %v: %v",
	// 			where, err)
	// 		ircMsg(msg)
	// 		log.Print(msg)
	// 	}
	// }("repoToNonMergeCommit")
	defer func() {
		if err := recover(); err != nil {
			msg := fmt.Sprintf("Recovered from error in repoToNonMergeCommit: %v",
				err)
			ircMsg(msg)
			log.Print(msg)
		}
	}()

	const GIT_COMMAND = "git log -1"
	const GIT_COMMAND_IF_MERGE = "git log -2"

	output := gitCommandToOutput(fullRepoPath, GIT_COMMAND)
	if output == "" {
		return GitCommit{}
	}
	if strings.Contains(output, "\nMerge:") {
		commitStr := gitCommandToOutput(fullRepoPath, GIT_COMMAND_IF_MERGE)
		// Find beginning of second-to-last commit (which is the most
		// recent non-merge commit)
		ndx := strings.Index(commitStr, "\ncommit ")
		output = commitStr[ndx+1:]
	}
	lines := strings.SplitN(output, "\n", 4)

	// Save each line indivudually
	commitLine := lines[0]
	authorLine := lines[1]
	dateLine := lines[2]
	commitMsg := strings.Replace(lines[3], "\n", "", -1)[len("    "):]

	// Process lines

	// Parse out commit hash
	hash := strings.Split(commitLine, " ")[1]

	// Parse out author info
	authorInfo := authorLine[len("Author: "):]
	tokens := strings.Split(authorInfo, " ")
	// `<email@domain.com>` (with angle brackets)
	bracketEMAILbracket := tokens[len(tokens)-1]
	n := len(bracketEMAILbracket)
	// `email@domain.com` (without angle brackets)
	authorEmail := bracketEMAILbracket[1:n-1]
	// All tokens except for last (which is the email)
	authorNames := tokens[:len(tokens)-1]
	author := strings.Join(authorNames, " ")

	date := dateLine[len("Date:   "):]

	commit := GitCommit {
	Hash: hash,
	Author: author,
	Email: authorEmail,
	Date: date,
	Message: commitMsg,
	Repo: repoName,
	}
	return commit
}


func gitCommandToOutput(fullRepoPath, command string) string {
    args := strings.Split(command, " ")
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = fullRepoPath  // Where cmd is run from
	output, err := cmd.Output()
	if err != nil {
		log.Printf("No repo found at '%v'(?). Error: %v\n",
			fullRepoPath, err)
		// Search ${repoName}/bare then ${repoName}_site for desired repo

		// Try bare/ if neither has been tried
		// (Should work on server)
		if !strings.HasSuffix(fullRepoPath, "/bare") &&
			!strings.HasSuffix(fullRepoPath, "_site") {
			return gitCommandToOutput(fullRepoPath + "/bare", command)
		}
		// Try _site/ after trying bare/
		// (Usually necessary locally)
		if !strings.HasSuffix(fullRepoPath, "_site") {
			// Ignore last len("/bare") chars
			fullRepoPath = fullRepoPath[:len(fullRepoPath)-len("/bare")]
			return gitCommandToOutput(fullRepoPath + "_site", command)
		}

		// Now both /bare and _site have been tried. Giving up.

		if repoList := listRepos(); repoList != "" {
			ircMsg("Repo not found. Options (probably): " + repoList)
		}
		return ""
	}
	return string(output)
}


func gitRepoNameToFullPath(repoName string) string {
	// Alternatives to this I can think of: change global
	// REPO_BASE_PATH from const into var (global variables == bad!)
	repoBase := REPO_BASE_PATH[:]

	// If repoName begins with /, user is giving absolute path to repo
	if strings.HasPrefix(repoName, "/") {
		repoBase = ""
	}
    return repoBase + repoName
}


func gitRepoDataParser(repoName string) GitCommit {
	defer func() {
		if err := recover(); err != nil {
			msg := fmt.Sprintf("Recovered from error in gitRepoDataParser: %v",
				err)
			ircMsg(msg)
			log.Print(msg)
		}
	}()
	fullRepoPath := gitRepoNameToFullPath(repoName)
	commit := repoToNonMergeCommit(fullRepoPath, repoName)
	return commit
}

// gitListener listens on localhost:GIT_PORT for git repo names
// coming from git post-receive hooks, then
func gitListener() {
	defer func() {
		if err := recover(); err != nil {
			msg := fmt.Sprintf("Recovered from error in gitListener: %v",
				err)
			ircMsg(msg)
		}
	}()

	// Create TCP connection
	addr := "127.0.0.1:" + GIT_PORT
	tcpAddr, err := net.ResolveTCPAddr("tcp4", addr)
	checkError("net.ResolveTCPAddr", err)

	listener, err := net.ListenTCP("tcp", tcpAddr)
	checkError("net.ListenTCP", err)

	for {
		var buf [512]byte

		conn, err := listener.Accept()
		if err != nil {
			msg := fmt.Sprintf("Error in gitListener: %v\n", err)
			log.Printf(msg)
			ircMsg(msg)
			time.Sleep(2e9)
			continue
		}
		// Read n bytes
		n, err := conn.Read(buf[:])

		repoName := string(buf[:n])
		log.Printf("Repo name received: '%v'", repoName)

		gitCommit := gitRepoDataParser(repoName)
		// TODO: Complain to go-nuts mailing list
		empty := GitCommit{}
		if gitCommit != empty {
			msg := fmt.Sprintf(`%v pushed to %v: "%v"`,
				gitCommit.Author, repoName, gitCommit.Message)
			ircMsg(msg)
		}
		// Link us to all tickets referenced in commit message
		getIDs := regexp.MustCompile(`#[0-9]+`)
		for _, id := range getIDs.FindAllString(gitCommit.Message, -1) {
			ircMsg(REDMINE_URL + "/issues/" + id[1:])
		}
		conn.Close()
	}
	return
}

func listRepos() string {
	result, err := ioutil.ReadFile(REPO_BASE_PATH + REPO_INDEX_FILE)
	if err != nil {
		fmt.Printf("%v\n", "No index file found")

		// List directory contents instead

		// TODO: Look for subdirectories of REPO_BASE_PATH containing
		// .git/ -- wait for it -- subdirectories

		// Only include directory names
		// TODO: Assumes sub-directories do not include spaces
		// cmd := exec.Command("ls -l | grep ^d | awk '{print $9}'")

		// WTF? FIXME: Use Go's own directory listing
		// capabilities... _Duh_...
		cmd := exec.Command("ls")
		cmd.Dir = REPO_BASE_PATH
		// If no error, `result` used after this block
		result, err = cmd.Output()
		if err != nil {
			fmt.Printf("%v\n", err)
			msg := "Can't run 'ls'?! Somebody screwed up REPO_BASE_PATH..."
			fmt.Printf("%v\n", msg)
			ircMsg(msg)
			return ""
		}
	}
	// `result` came from .index file or REPO_BASE_PATH dir listing
	repoStr := string(result)

	// No valid repo supplied, no .index file, and REPO_BASE_PATH
	// doesn't exist(?). TODO: double-check when it's not 6:30am after
	// you've stayed up all night...
	if repoStr == "" {
		return ""
	}

	repoNames := strings.Split(repoStr, "\n")

	// Remove last repo in repoNames if it's empty (comes from
	// trailing newline in REPO_INDEX_FILE)
	if repoNames[len(repoNames)-1] == "" {
		repoNames = repoNames[:len(repoNames)-1]
	}
	// Remove potential repo names that include a "." (those are
	// files, not directories)
	for ndx, name := range repoNames {
		if !strings.HasSuffix(name, ".git") &&
			strings.Contains(name, ".") {
			// Remove this (non-)"repo" from repoNames
			repoNames = append(repoNames[:ndx], repoNames[ndx+1:]...)
		}
	}
	return strings.Join(repoNames, ", ")
}

//
// Accept GitHub Webhook data (qua JSON file) on port 7777
//

// TODO: Define JSON "template" as struct to capture its structure?

func webhookHandler(w http.ResponseWriter, req *http.Request) {
	if req.Method != "POST" {
		info := fmt.Sprintf("req:\n\n%+#v\n", req)
		fmt.Printf("%v\n", info)
		// w.Write([]byte(info))
		return
	}
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		fmt.Printf("Error in webhookHandler: %v\n", err)
		return
	}
	req.Body.Close()
	values, err := url.ParseQuery(string(body))
	if err != nil {
		fmt.Printf("Error parsing body.\nbody == %s\n\nError: %v",
			body, err)
		return
	}
	// Parsing GitHub-specific format
	data := values["payload"][0]
	commit := webhookDataToGitCommit(data)
	if VERBOSE {
		irc <- fmt.Sprintf(`%s just pushed to %s on GitHub: "%s"`,
			commit.Author, commit.Repo, commit.Message)
	}
	fmt.Printf("Updating local GitHub repo '%v'\n", commit.Repo)
	updateLocalGitHubRepo(commit.Repo)
	return
}

// webhookListener listens on WEBHOOK_PORT (default: 7777) for JSON
// HTTP POSTs, parses the relevant data, then sends it over a channel
// to a function waiting for strings to echo into IRC_CHANNEL
func webhookListener() {
	http.HandleFunc("/webhook", webhookHandler)
	// err := http.ListenAndServeTLS(":"+WEBHOOK_PORT, "cert.pem", "key.pem", nil)
	err := http.ListenAndServe(":"+WEBHOOK_PORT, nil)
	if err != nil {
		fmt.Printf("Error in webhookListener ListenAndServe: %v\n", err)
		time.Sleep(2e9)
	}
}

func webhookDataToGitCommit(data string) GitCommit {
	commit := GitCommit{}
	// data string -> []byte -> Unmarshal into interface{}
	b := []byte(data)
	var m interface{}
	err := json.Unmarshal(b, &m)
	if err != nil {
		msg := "Couldn't unmarshal WebHook data: " + data
		ircMsg(msg)
		log.Printf(msg)
		return commit
	}
	// Parse these pieces:
	// commit.Author    = payload[pusher][name]
	// commit.Email     = payload[pusher][email]
	// // commit.Author    = payload[head_commit][author][username]
	// // commit.Email     = payload[head_commit][author][email]
	// commit.Message   = payload[head_commit][message]
	// commit.Repo      = payload[repository][name]
	// commit.RepoOwner = payload[repository][owner][name]
	payload := m.(map[string]interface{})
	if VERBOSE { fmt.Printf("payload == %+v\n", payload) }
	for k, v := range payload {
		if k == "pusher" {
			pusher := v.(map[string]interface{})
			commit.Author = fmt.Sprintf("%v", pusher["name"])
			commit.Email = fmt.Sprintf("%v", pusher["email"])
		}
		if k == "head_commit" {
			head_commit := v.(map[string]interface{})
			msg := fmt.Sprintf("%v", head_commit["message"])
			commit.Message = strings.Replace(msg, "\n", "    ", -1)
		}
		if k == "repository" {
			repository := v.(map[string]interface{})
			commit.Repo = fmt.Sprintf("%v", repository["name"])
			repoOwner := repository["owner"].(map[string]interface{})
			commit.RepoOwner = fmt.Sprintf("%v", repoOwner["name"])
		}
	}
	return commit
}

func updateLocalGitHubRepo(repoName string) {
	fullRepoPath := LOCAL_GITHUB_REPOS + repoName
	const command = "git fetch"
	output := gitCommandToOutput(fullRepoPath, command)
	fmt.Printf(`Output from fetch:
========
%v
========
`, output)
	// ircMsg has type `func(string)`...
	f := ircMsg
	if !VERBOSE {
		// ...and so does privMsgOwner
		f = privMsgOwner
	}
	if strings.Contains(output, "\nUnpacking objects") {
		f("Successfully ~updated (fetched) " + fullRepoPath +
			" repo on " + THIS_SERVER_NAME)
	} else {
		// Pass for now. TODO: Make this more robust; get rid of all
		// misleading errors.

		// f("Failed to pull from GitHub to " + fullRepoPath +
		// 	" repo on " + THIS_SERVER_NAME)
	}
}
