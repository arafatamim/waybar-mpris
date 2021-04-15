## waybar-mpris

<p align="center">
    <img src="images/cropped.gif" style="width: 100%;" alt="bar gif"></img>
</p>

a waybar component/utility for displaying and controlling MPRIS2 compliant media players individually, inspired by [waybar-media](https://github.com/yurihs/waybar-media).

MPRIS2 is widely supported, so this component should work with:

- Chrome/Chromium
- Firefox (Limited support with `media.hardwaremediakeys.enabled = true` in about:config)
- Other browsers (using KDE Plasma Integration)
- VLC
- Spotify
- Noson
- mpd (with [mpDris2](https://github.com/eonpatapon/mpDris2))
- Most other music/media players

## Install

Available on the AUR as [waybar-mpris-git](https://aur.archlinux.org/packages/waybar-mpris-git/) (Thanks @nichobi!)

`go get git.hrfee.pw/hrfee/waybar-mpris` will compile from source and install.

You can also download a precompiled binaries from [here](https://builds2.hrfee.pw/view/hrfee/waybar-mpris).

## Issues

Stick them on [mpris2client](https://github.com/hrfee/mpris2client) or the [og](https://github.com/hrfee/waybar-mpris) repository (both on github) if you can't make an account here.

## Usage

When running, the program will pipe out json in waybar's format. Add something like this to your waybar `config.json`:

```
"custom/waybar-mpris": {
    "return-type": "json",
    "exec": "waybar-mpris --position --autofocus",
    "on-click": "waybar-mpris --send toggle",
    // This option will switch between players on right click.
        "on-click-right": "waybar-mpris --send player-next",
    // The options below will switch the selected player on scroll
        // "on-scroll-up": "waybar-mpris --send player-next",
        // "on-scroll-down": "waybar-mpris --send player-prev",
    // The options below will go to next/previous track on scroll
        // "on-scroll-up": "waybar-mpris --send next",
        // "on-scroll-down": "waybar-mpris --send prev",
    "escape": true,
},
```

```
Usage of waybar-mpris:
      --autofocus          Auto switch to currently playing music players.
      --interpolate        Interpolate track position (helpful for players that don't update regularly, e.g mpDris2)
      --order string       Element order. (default "SYMBOL:ARTIST:ALBUM:TITLE:POSITION")
      --pause string       Pause symbol/text to use. (default "\uf8e3")
      --play string        Play symbol/text to use. (default "▶")
      --position           Show current position between brackets, e.g (04:50/05:00)
      --replace            Replace any running instances
      --send string        send command to already runnning waybar-mpris instance. (options: player-next/player-prev/next/prev/toggle)
      --separator string   Separator string to use between artist, album, and title. (default " - ")
```

- Modify the order of components with `--order`. `SYMBOL` is the play/paused icon or text, `POSITION` is the track position (if enabled), other options are self explanatory.
- `--play/--pause` specify the symbols or text to display when music is paused/playing respectively.
- `--separator` specifies a string to separate the artist, album and title text.
- `--autofocus` makes waybar-mpris automatically focus on currently playing music players.
- `--position` enables the display of the track position.
- `--interpolate` increments the track position every second. This is useful for players (e.g mpDris2) that don't regularly update the position.
- `--replace`: By default, new instances will attach to the existing one so that the output is identical. This lets this instance replace any others running. It isn't recommended.
- `--send` sends commands to an already running waybar-mpris instance via a unix socket. Commands:
  - `player-next`: Switch to displaying and controlling next available player.
  - `player-prev`: Same as `player-next`, but for the previous player.
  - `next/prev`: Next/previous track on the selected player.
  - `toggle`: Play/pause.
  - You can also bind these commands to Media keys in your WM config.

## Notes

Originally forked from [https://git.hrfee.pw/hrfee/waybar-mpris](https://git.hrfee.pw/hrfee/waybar-mpris)
