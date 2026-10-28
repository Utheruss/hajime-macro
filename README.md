# Hajime Macro

A small Windows utility that automates the stat-training minigames in the Roblox game **Hajime Rising**.
It only **reads the screen and emulates mouse/keyboard input**, like a human would. It does not touch the game's memory and does not inject anything.

> **Made entirely by Claude (Anthropic).** The whole program, tests and this README were written by an AI at the request of a player.

## Features
- Stats: Arm strength, Leg strength, Torso endurance (hold the mouse button inside the box), Punch speed, Jump rope (click when the indicator crosses the centre), Punch sharpness (alternating left/right click on the bag).
- Each stat is bound to its training method automatically.
- Waits for stamina to refill to 100%, waits 1 second, then presses `E` to start again.
- **F6** starts/stops. Second press stops the training and gives control back.
- Pauses automatically while the Roblox window is not active.
- Works on any resolution and Windows UI scale (everything is detected from the screen).
- Stat cap reached: the macro beeps, stops the training (like F6) and does not start it again.
- Auto energy / food / water (three checkboxes, on by default): when energy < 10% or food/water <= 5%, the macro stops the training with `E`, then uses the item from your hotbar and goes back to training after full stamina (+1 s):
  - energy: key `4`, left click, left click
  - food: key `2`, left click x4
  - water: key `3`, left click x2

  (0.25 s between actions; for Punch speed / Punch sharpness the gloves, key `1`, are re-equipped afterwards. If a bar is empty again right after use, e.g. no items left, the macro does not retry and shows a warning.)
- Red "WARNING" with a beep (several can show at once): stat cap reached, energy below 10%, gloves not equipped (Punch speed / Punch sharpness). Cap and energy are re-checked at the start of every training.
- Languages: English (default), Русский, Español. Built-in usage guide (the **i** button).
- Adjustable click lead time and punch delay, session statistics, "Diagnostics" button (saves a screenshot, a detection report and a log of the latest macro actions). The window also shows the energy / food / water levels the macro reads.

## Usage
1. Download `HajimeMacro.exe` from the Releases page (or build it yourself, see below).
2. Start Roblox, open the training of the stat you want, pick the same stat in the macro.
3. Press **F6** (or the button). Press **F6** again to stop.

More detail is in the in-app guide (the **i** button).

## Build
Requires [Go](https://go.dev/) 1.24+. No other dependencies.

```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -H=windowsgui" -o HajimeMacro.exe .
```

Run the tests (on any OS): `go test ./...`

## Permission from the game's staff
This is not an official statement, only what was said in the game's Discord: the game's owner replied that they don't mind macros "as long as you don't hack", and a developer said publishing a macro is fine (the game is going to be revamped anyway). This can change at any time. Always follow the game's rules.

## Disclaimer
Use at your own risk. This project is not affiliated with Roblox or the developers of Hajime Rising. Because the program emulates input and reads the screen, some antiviruses may flag the unsigned exe as a false positive; you can read the source and build it yourself.

## Bugs and feedback
Found a bug? Open an **Issue** in this repository. Attaching the files from the "Diagnostics" button (`hajime_diag.png`, `hajime_diag.txt`) helps a lot.

## Кратко по-русски
Макрос для автоматической прокачки статов в Hajime Rising. Он только смотрит на экран и эмулирует мышь и клавиатуру, в игру не лезет. Полностью написан Claude (ИИ). Выбери стат, нажми F6. Об ошибках пишите в Issues. Используешь на свой риск.
