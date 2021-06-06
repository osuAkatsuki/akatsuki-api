[![Discord](https://discordapp.com/api/guilds/365406575893938177/widget.png?style=shield)](https://discord.gg/5cBtMPW)

# akatsukiapi

This is the source code for Akatsuki's API.

- Origin: https://git.zxq.co/ripple/rippleapi
- Mirror: https://github.com/osuripple/api

# Setting up
`go get -u -f -d github.com/osuAkatsuki/akatsuki-api`

`cd $GOPATH/src/github.com/osuAkatsuki/akatsuki-api`

Download all dependencies:

  `go mod download`


Compile:

  `go build`

Run API
- On Windows:

  `akatsuki-api` (located in `./akatsuki-api.exe`)
- On Linux:

  `./akatsuki-api`

Then configure in `api.conf` and run the API again.
