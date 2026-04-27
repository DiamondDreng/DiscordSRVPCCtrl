# DiscordSRVPCCtrl
> **⚠️ DISCLAIMER: EDUCATIONAL PURPOSES ONLY**  
> This program is created ONLY for educational and research purposes.  
> This program can be used to execute powershell commands directly on a windows system.  
> Use of this program on machines without explicit authorization may violate laws and Terms of Service.  
> I am not responsible for any misuse. Use at your own risk and make sure you comply with all applicable laws, terms and conditions.  


### **Why**
I wanted to make this program because it wanted an easy way of running powershell commands on remote/virtual machines.  
Then i thought of the Minecraft server plugin called DiscordSRV that lets you see minecraft chat, joins, console and even run commands on the server directly from discord.  
If you didn't notice already, i got the name from DiscordSRV. And so the full name of the program is: Discord Server Personal Computer Controller.  


### **How To Use/Setup**
Im bad at tutorials so you might need to search some stuff up  
**Discord Bot Setup**:
1. Create a Bot through the Discord developer portal
2. Enable all 3 gateway intents
3. Get the Bot Token by clicking Reset Token (save it for later)
4. Enable Developer mode in discord (search it up. im not explaining)
5. Right click your discord server to use for controlling computers from and click copy Server ID
6. Replace "DISCORD_BOT_TOKEN" and "DISCORD_SERVER_ID" in the config.ENV with your Bot token and server id
7. Transfer config.env and discord-pc-control.exe to the windows machine and run "discord-pc-control.exe --install" to install the program
8. Restart the machine. You should get a message in your discord server


**Optional**:
Uncomment "DISCORD_ALLOWED_USERS" in config.env and add comma seperated userID's. Only users with those id's can use commands.  
Change "DISCORD_CATEGORY" to something to make computers automatically create/add themselves to that category


### **How To Build**:
I use Arch so just download it precompiled or figure it out on your own if you wanna do this fancy stuff on windows  
**Arch**: Just run this command from your terminal in the directory where you have the files (main.go, etc)
```
GOOS=windows GOARCH=amd64 go build -ldflags "-H windowsgui" -o discord-pc-control.exe .
```

Pls make an issue if you have any issues, improvements or just questions.  

**remember to star** ⭐
