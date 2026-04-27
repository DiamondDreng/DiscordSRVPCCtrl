package main

// First run on Windows (registers startup task):
// discord-pc-control.exe --install

// Remove startup task:
// discord-pc-control.exe --uninstall

// config.env (place next to the exe):
// DISCORD_BOT_TOKEN=your_bot_token
// DISCORD_SERVER_ID=your_server_id
// DISCORD_ALLOWED_USERS=comma,separated,discord,user,ids  (optional)
// DISCORD_CATEGORY=PC Control  (optional category name, default none)

// Variables like DISCORD_ALLOWED_USERS can be removed, empty or commented to disable that feature
// ( e.gs: if DISCORD_ALLOWED_USERS is commented, anyone with access to the channels can execute commands)

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
)

// ── Config ────────────────────────────────────────────────────────────────────

var (
	BotToken     string
	ServerID     string
	AllowedUsers string
	CategoryName string // optional: group all PC channels under one category
)

const taskName = "DiscordPCControl"

// ── Entry point ───────────────────────────────────────────────────────────────

func main() {
	loadConfig()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--install":
			installTask()
		case "--uninstall":
			uninstallTask()
		default:
			showDialog("discord-pc-control",
				"Usage:\n\n"+
					"  (no flag)     — run the bot\n"+
					"  --install     — register Windows startup task\n"+
					"  --uninstall   — remove startup task")
		}
		return
	}

	runBot()
}

// ── Bot ───────────────────────────────────────────────────────────────────────

func runBot() {
	if BotToken == "" || ServerID == "" {
		showFatalDialog("discord-pc-control: missing config",
			"DISCORD_BOT_TOKEN and DISCORD_SERVER_ID must be set in config.env next to the exe.")
		os.Exit(1)
	}

	dg, err := discordgo.New("Bot " + BotToken)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// We need GuildMessages to receive commands and Guilds to list/create channels.
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsGuilds

	if err = dg.Open(); err != nil {
		log.Fatalf("Error opening Discord connection: %v", err)
	}
	defer dg.Close()

	// Resolve (or create) the channel for this machine.
	channelID, err := ensureChannel(dg)
	if err != nil {
		log.Fatalf("Could not resolve channel: %v", err)
	}
	log.Printf("Using channel ID %s", channelID)

	allowed := buildAllowedSet()

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		handleMessage(s, m, channelID, allowed)
	})

	announce(dg, channelID)

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	dg.ChannelMessageSend(channelID, "🔴 **PC going offline** (bot shutting down)")
	log.Println("Shutting down.")
}

// ── Channel resolver ──────────────────────────────────────────────────────────
// Returns the ID of the channel whose name matches this machine's hostname,
// creating it (and optionally a category) if it doesn't already exist.

// discordChannelName converts a hostname to a valid Discord channel name:
// lowercase, spaces→hyphens, strip anything that isn't a-z0-9 or hyphen.
var invalidChars = regexp.MustCompile(`[^a-z0-9\-]`)

func discordChannelName(hostname string) string {
	name := strings.ToLower(hostname)
	name = strings.ReplaceAll(name, " ", "-")
	name = strings.ReplaceAll(name, "_", "-")
	name = invalidChars.ReplaceAllString(name, "")
	// Collapse repeated hyphens and trim
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")
	if len(name) > 100 { // Discord channel name limit
		name = name[:100]
	}
	if name == "" {
		name = "pc-control"
	}
	return name
}

func ensureChannel(dg *discordgo.Session) (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-pc"
	}
	chanName := discordChannelName(hostname)
	log.Printf("Looking for channel #%s in server %s", chanName, ServerID)

	channels, err := dg.GuildChannels(ServerID)
	if err != nil {
		return "", fmt.Errorf("cannot fetch guild channels: %w", err)
	}

	// Check if the channel already exists.
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildText && ch.Name == chanName {
			log.Printf("Found existing channel #%s (%s)", chanName, ch.ID)
			return ch.ID, nil
		}
	}

	// Channel doesn't exist — optionally find/create a category first.
	var parentID string
	if CategoryName != "" {
		parentID, err = ensureCategory(dg, channels)
		if err != nil {
			log.Printf("Warning: could not create category: %v (will create channel without one)", err)
			parentID = ""
		}
	}

	// Create the channel.
	log.Printf("Channel #%s not found — creating it", chanName)
	data := discordgo.GuildChannelCreateData{
		Name:  chanName,
		Type:  discordgo.ChannelTypeGuildText,
		Topic: fmt.Sprintf("Remote control channel for %s — managed by discord-pc-control", hostname),
	}
	if parentID != "" {
		data.ParentID = parentID
	}

	newCh, err := dg.GuildChannelCreateComplex(ServerID, data)
	if err != nil {
		return "", fmt.Errorf("cannot create channel #%s: %w", chanName, err)
	}

	log.Printf("Created channel #%s (%s)", chanName, newCh.ID)
	return newCh.ID, nil
}

// ensureCategory finds or creates the category named CategoryName.
func ensureCategory(dg *discordgo.Session, channels []*discordgo.Channel) (string, error) {
	targetName := strings.ToLower(CategoryName)
	for _, ch := range channels {
		if ch.Type == discordgo.ChannelTypeGuildCategory &&
			strings.ToLower(ch.Name) == targetName {
			return ch.ID, nil
		}
	}
	// Create it.
	cat, err := dg.GuildChannelCreateComplex(ServerID, discordgo.GuildChannelCreateData{
		Name: CategoryName,
		Type: discordgo.ChannelTypeGuildCategory,
	})
	if err != nil {
		return "", err
	}
	log.Printf("Created category '%s' (%s)", CategoryName, cat.ID)
	return cat.ID, nil
}

// ── Startup announcement ──────────────────────────────────────────────────────

func announce(dg *discordgo.Session, channelID string) {
	hostname, _ := os.Hostname()
	t := time.Now().Format("Mon 02 Jan 2006 • 15:04:05")

	embed := &discordgo.MessageEmbed{
		Title:       "🟢 PC Online",
		Description: fmt.Sprintf("**%s** is now online and ready for remote commands.", hostname),
		Color:       0x57F287,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "🕒 Time", Value: t, Inline: true},
			{Name: "💻 Host", Value: hostname, Inline: true},
			{Name: "📋 Commands",
				Value: "`!ps <command>` — run PowerShell\n`!uptime` — system uptime\n`!sysinfo` — system info\n`!help` — all commands",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "discord-pc-control • Windows"},
	}

	if _, err := dg.ChannelMessageSendEmbed(channelID, embed); err != nil {
		log.Printf("Failed to send announcement: %v", err)
	}
}

// ── Message handler ───────────────────────────────────────────────────────────

func handleMessage(s *discordgo.Session, m *discordgo.MessageCreate, channelID string, allowed map[string]bool) {
	if m.Author.ID == s.State.User.ID || m.ChannelID != channelID {
		return
	}
	if len(allowed) > 0 && !allowed[m.Author.ID] {
		s.ChannelMessageSend(channelID,
			fmt.Sprintf("⛔ <@%s> you are not authorised to run commands.", m.Author.ID))
		return
	}

	content := strings.TrimSpace(m.Content)

	switch {
	case content == "!help":
		sendHelp(s, channelID)

	case content == "!uptime":
		out, err := runPS(`(Get-Date) - (gcim Win32_OperatingSystem).LastBootUpTime |` +
			` Select-Object -ExpandProperty TotalHours |` +
			` ForEach-Object { '{0:N1} hours' -f $_ }`)
		replyOutput(s, m, channelID, "⏱️ Uptime", out, err)

	case content == "!sysinfo":
		out, err := runPS(
			`$os  = gcim Win32_OperatingSystem;` +
				` $cpu = gcim Win32_Processor | Select -First 1;` +
				` "OS:  " + $os.Caption + [char]10 +` +
				` "CPU: " + $cpu.Name   + [char]10 +` +
				` "RAM: " + [math]::Round($os.TotalVisibleMemorySize/1MB,1) + " GB total, " +` +
				`           [math]::Round($os.FreePhysicalMemory/1MB,1)     + " GB free"`)
		replyOutput(s, m, channelID, "🖥️ System Info", out, err)

	case strings.HasPrefix(content, "!ps "):
		cmd := strings.TrimPrefix(content, "!ps ")
		log.Printf("PS from %s: %s", m.Author.Username, cmd)
		s.ChannelTyping(channelID)
		out, err := runPS(cmd)
		replyOutput(s, m, channelID, "💻 PowerShell", out, err)

	case content == "!ps":
		s.ChannelMessageSend(channelID,
			"⚠️ Usage: `!ps <command>`\nExample: `!ps Get-Process | Select -First 5 | Format-Table Name,CPU`")
	}
}

// ── PowerShell runner ─────────────────────────────────────────────────────────

func runPS(command string) (string, error) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	
	// Hide the PowerShell window on Windows
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if result == "" && err == nil {
		result = "(no output)"
	}
	return result, err
}

// ── Discord reply helpers ─────────────────────────────────────────────────────

func replyOutput(s *discordgo.Session, m *discordgo.MessageCreate, channelID, title, output string, cmdErr error) {
	color, status := 0x57F287, "✅ Success"
	if cmdErr != nil {
		color = 0xED4245
		status = fmt.Sprintf("❌ Error: %v", cmdErr)
	}
	const maxLen = 1900
	if len(output) > maxLen {
		output = output[:maxLen] + "\n… (truncated)"
	}
	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: color,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Status", Value: status},
			{Name: "Output", Value: fmt.Sprintf("```\n%s\n```", output)},
		},
		Footer: &discordgo.MessageEmbedFooter{Text: "Requested by " + m.Author.Username},
	}
	if _, err := s.ChannelMessageSendEmbedReply(channelID, embed, m.Reference()); err != nil {
		log.Printf("Failed to send reply: %v", err)
	}
}

func sendHelp(s *discordgo.Session, channelID string) {
	s.ChannelMessageSendEmbed(channelID, &discordgo.MessageEmbed{
		Title: "📖 discord-pc-control",
		Color: 0x5865F2,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "`!ps <command>`", Value: "Run any PowerShell command.\nExample: `!ps Get-Date`"},
			{Name: "`!uptime`", Value: "Show how long the PC has been on."},
			{Name: "`!sysinfo`", Value: "Show OS, CPU and RAM info."},
			{Name: "`!help`", Value: "Show this message."},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Tip: !ps shutdown /s /t 0  •  !ps Restart-Computer -Force",
		},
	})
}

// ── Task Scheduler install / uninstall ───────────────────────────────────────

func exePath() string {
	path, err := os.Executable()
	if err != nil {
		log.Fatalf("Cannot determine exe path: %v", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return path
}

func installTask() {
	exe := exePath()
	psExe := strings.ReplaceAll(exe, `'`, `''`)

	ps := fmt.Sprintf(`
$exe  = '%s'
$name = '%s'

Unregister-ScheduledTask -TaskName $name -Confirm:$false -ErrorAction SilentlyContinue

$action    = New-ScheduledTaskAction -Execute $exe -WorkingDirectory (Split-Path $exe)
$trigger   = New-ScheduledTaskTrigger -AtLogOn
$settings  = New-ScheduledTaskSettingsSet `+"`"+`
                 -ExecutionTimeLimit  (New-TimeSpan -Hours 0) `+"`"+`
                 -RestartCount        3 `+"`"+`
                 -RestartInterval     (New-TimeSpan -Minutes 1) `+"`"+`
                 -StartWhenAvailable
$principal = New-ScheduledTaskPrincipal `+"`"+`
                 -UserId    "$env:USERDOMAIN\$env:USERNAME" `+"`"+`
                 -LogonType Interactive

Register-ScheduledTask `+"`"+`
    -TaskName   $name `+"`"+`
    -Action     $action `+"`"+`
    -Trigger    $trigger `+"`"+`
    -Settings   $settings `+"`"+`
    -Principal  $principal `+"`"+`
    -Description 'Discord PC Control — starts at login' | Out-Null

Write-Host 'OK'
`, psExe, taskName)

	out, err := runPS(ps)
	if err != nil || !strings.Contains(out, "OK") {
		showFatalDialog("Install failed",
			fmt.Sprintf("Could not register scheduled task.\n\nOutput:\n%s\n\nError: %v", out, err))
		os.Exit(1)
	}

	showDialog("Installed ✅",
		fmt.Sprintf("Startup task '%s' registered.\n\nThe bot will start automatically on next login.\n\nExe: %s",
			taskName, exe))
}

func uninstallTask() {
	ps := fmt.Sprintf(`
Unregister-ScheduledTask -TaskName '%s' -Confirm:$false -ErrorAction SilentlyContinue
Write-Host 'OK'
`, taskName)

	out, err := runPS(ps)
	if err != nil || !strings.Contains(out, "OK") {
		showFatalDialog("Uninstall failed",
			fmt.Sprintf("Could not remove scheduled task.\n\nOutput:\n%s\n\nError: %v", out, err))
		os.Exit(1)
	}
	showDialog("Uninstalled ✅", fmt.Sprintf("Startup task '%s' has been removed.", taskName))
}

// ── Config loader ─────────────────────────────────────────────────────────────

func loadConfig() {
	if exe, err := os.Executable(); err == nil {
		envPath := filepath.Join(filepath.Dir(exe), "config.env")
		if data, err := os.ReadFile(envPath); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				if parts := strings.SplitN(line, "=", 2); len(parts) == 2 {
					k := strings.TrimSpace(parts[0])
					v := strings.TrimSpace(parts[1])
					if os.Getenv(k) == "" {
						os.Setenv(k, v)
					}
				}
			}
		}
	}

	BotToken = os.Getenv("DISCORD_BOT_TOKEN")
	ServerID = os.Getenv("DISCORD_SERVER_ID")
	AllowedUsers = os.Getenv("DISCORD_ALLOWED_USERS")
	CategoryName = os.Getenv("DISCORD_CATEGORY")
}

func buildAllowedSet() map[string]bool {
	set := map[string]bool{}
	for _, id := range strings.Split(AllowedUsers, ",") {
		if id = strings.TrimSpace(id); id != "" {
			set[id] = true
		}
	}
	return set
}

// ── Windows message boxes (cross-compiles from Linux, no CGO) ─────────────────

func showDialog(title, message string) {
	runMsgBox(escapePS(title), escapePS(message), "Information")
}

func showFatalDialog(title, message string) {
	runMsgBox(escapePS(title), escapePS(message), "Error")
}

func runMsgBox(title, message, icon string) {
	ps := fmt.Sprintf(
		`Add-Type -AssemblyName PresentationFramework; `+
			`[System.Windows.MessageBox]::Show('%s','%s','OK','%s') | Out-Null`,
		message, title, icon)
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}
	cmd.Run()
}

func escapePS(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `''`)
	return s
}
