# KeyClick Tracker

A quiet, private Windows app that counts your key presses, mouse clicks and scrolls, so you can see the shape of your day.

**It only stores counts.** It never records what you type, where your cursor goes, or any text. Everything stays on your computer in `%LOCALAPPDATA%\KeyScope`, and nothing is sent anywhere.

## What it does

- **Live numbers** for key presses, clicks and scrolls, with sliding digits.
- **Typing and clicking speed** (WPS / WPM / WPH, CPS / CPM / CPH), live and per period.
- **Overview, Keyboard, Mouse, Speed and History** pages with a heat map, goals, streaks and milestones.
- **Analytics** (new in 1.2.0): an engine that studies your counts as they happen and keeps charts and findings up to date. It has activity bars from hourly to yearly, a trend line with a moving average, an hourly rhythm and a 24-hour clock, a weekly rhythm, an hour-by-weekday heat map, a year-of-days calendar, an activity mix, and a breakdown of how you type. It also has a live insight list, a profile, and four scores.
- **Floating live box** that stays on top of other windows, with ten colour themes.
- **Data and backup**: export and import as JSON, TXT or Markdown, with automatic backups before updates.

## Version 1.2.0 (2026-11-15)

**New**

- Analytics page with the charts, insight engine and profile described above. The page is driven entirely by your counts; no text is stored or read.
- Insight engine findings: pace today against a normal day, your sharpest three hours, chronotype, consistency, week-on-week momentum, unusual days, weekday and weekend habits, keyboard and mouse style, correction rate, streak, and biggest day.
- Activity score, consistency and momentum, plus a projected total for today, with sliding numbers.
- Ten colour themes for the floating live box, separate from the app theme.
- Activity glow on the stat boxes: keys in blue, clicks in lilac, scrolls in green. The All activity box glows for any event.
- Official typefaces in Appearance (Times New Roman, Arial, Calibri, Cambria, Georgia, Book Antiqua, Palatino Linotype, Verdana, Tahoma, Comic Sans MS), with other families under "Other typefaces".
- Contact us in About: Instagram, Telegram and Discord.
- Real word count: a hidden engine counts words as you type, keeping only a counter and an on/off flag.

**Improved**

- The ✕ on the live box is smaller and no longer covers the numbers.
- Export and import accept only JSON, TXT and MD. Any other file type shows a warning and changes nothing.
- All-time records moved from Overview to Analytics.

**Fixed**

- Keys, clicks and scrolls were sometimes not counted in some apps, so speed stayed at 0. Input is now captured on its own thread and the listener is re-armed every three seconds.
- A key not counted after a missed release is now counted when pressed again.
- The floating live box could disappear (saved position on a disconnected monitor, minimised, or behind other windows). It is now repaired automatically and stays on top.
- The floating box had no colour; it now has its own themes and rounded hover glow that follows its corners.
- The All-time records numbers slide smoothly and no longer stop updating.
- Typeface notifications no longer stack, and they appear in Appearance too, in your app theme colours.
- The Insight engine no longer blinks while you work: findings update in place.
- While the engine is still learning, its scores show a dash instead of a misleading number.

## Build

Requirements: Windows 10 or 11, and a Go toolchain (the project was built with Go 1.27.4).

```powershell
cd Native-Windows-Source
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

The build writes `KeyClick-Tracker-v1.2.0.exe` to the parent folder. Set `KEYCLICK_DEV=1` and `KEYCLICK_DATA_DIR` only for automated tests; a normal start never sets them.

## Project notes

- `MASTER_DEVELOPMENT.md` is the development record: every change, test and decision, with dates.
- `Original-Dashboard-Source` is the earlier dashboard source, kept for reference.
- `icons` holds the logo concepts.

## Status

Analytics is verified on synthetic data only. Results on long personal histories and in the light theme are still to be checked.
