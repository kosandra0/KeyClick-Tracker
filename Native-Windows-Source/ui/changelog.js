/* Update log shown in the app. Newest first. Add a new entry on every release. */
window.KC_CHANGELOG = [
  {
    version: '2.4.0', date: '2026-11-19', title: 'How long keys and buttons are held, scroll sessions, and an Applications page',
    intro: 'KeyClick Tracker now measures how long each key and mouse button is held, groups scrolling into sessions, and shows what each program received. Your existing statistics are kept. Durations start counting from this version; earlier days show Not recorded.',
    items: [
      ['new', 'Hold time for every key and mouse button, measured from press to release. Holding Ctrl, Shift and A gives three separate holds, left and right keys are kept apart, and keyboard auto-repeat does not create extra holds. The program in front when you pressed keeps the hold, even if you switch windows before releasing.'],
      ['new', 'A Held right now panel on Keyboard, Mouse and Applications shows the keys and buttons currently down with a live timer, and the scroll in progress.'],
      ['new', 'Scroll sessions: wheel events less than 0.4 seconds apart form one session. The time shown is an estimate from the gaps between wheel events (a wheel has no real hold), and a single wheel click shows no time at all.'],
      ['new', 'Input duration in Analytics: held time for keys and buttons, the most held keys, hold time for each mouse button, and the scroll session summary, for the range you choose.'],
      ['new', 'Input history in History: the newest 2,000 key holds, button holds and scroll sessions with search, sorting and paging.'],
      ['new', 'Applications page: every program that received your input, with its own icon, keys, clicks, scrolls, hold times, share and last input. Open a program to see its keyboard heat map, its mouse buttons and its scrolling. Only the program file name is kept, never window titles or typed text.'],
      ['new', 'Time held on the Overview: the Key presses and Mouse clicks cards show how long their input was held, and the Scroll events card shows the (estimated) scrolling time.'],
      ['new', 'Most used keys on the Keyboard page has Held and Average columns beside Presses, and every mouse button on the Mouse page shows how long its clicks were held. The Keyboard page shows only keys and the Mouse page only mouse buttons and scrolling.'],
      ['new', 'KeyClick Tracker is now listed in Applications like any other program, marked this app.'],
      ['improved', 'Smooth animations in the new screens: numbers roll like the Overview counters, held keys and buttons appear with a glow (a quick tap shows too), programs slide into place when the order changes, and a program opens and closes smoothly. They follow your Animations, Live numbers and Lite mode settings.'],
      ['fixed', 'Applications no longer jumps while you read it, and opening a program works with one click however often the page was visited.'],
      ['improved', 'Live numbers. Held time now counts up while you hold a key or button, everywhere it is shown (Overview, Keyboard, Mouse, Analytics, Applications): the number stays in place and its digits roll up, it is not redrawn or delayed, and a total never steps back or counts a hold twice when the hold ends.'],
      ['improved', 'Keys and mouse buttons glow. A key or button you are holding lights up on the Keyboard and Mouse pages, and in an opened program on Applications it glows (and pulses when pressed) in the program that has focus. The scrolling summary of a program is now a set of tiles, and the History day panel has headed lists.'],
      ['fixed', 'The History calendar tooltip no longer grows longer every few seconds.'],
      ['fixed', 'Numbers no longer vanish and reappear when you click or type. The Keyboard list of most used keys and the Mouse rows are now drawn with rows that stay in place: only their numbers roll, new rows rise in and rows that change order slide.'],
      ['improved', 'History: the By period table shows key-held, button-held and scroll time for every period, picking a day in the calendar opens a panel with the held time of that day, its most held keys and programs (a running day keeps counting live), and the input history rows slide in as new holds are recorded.'],
      ['improved', 'Less delay for held time. Totals tick in tenths of a second (a minute and more used to move once a second), the held keys are checked ten times a second and at once on every key press or click, and the digits roll in a fraction of a second.'],
      ['improved', 'Export and import (JSON, TXT and MD) now carry everything: hourly counts, daily totals, every key and button hold time, scroll sessions, the programs with their keys, buttons and times, and the newest 2,000 individual holds and scroll sessions with exact times. The text and Markdown reports also list the hold times and programs in readable form. Importing merges them without losing anything that exists only on this computer, and importing the same file twice changes nothing.'],
      ['changed', 'Hold time is added to the day it started and is not split at midnight. A hold open when you pause, sleep your PC or lose a key release for more than 15 minutes is dropped, never counted.'],
      ['changed', 'Totals add up every hold, so they can be more than the time that passed (several keys can be held at once). They are not time spent working.'],
    ],
  },
  {
    version: '2.3.0', date: '2026-11-19', title: 'Windows and Alt shortcuts are counted, scrolls per program, tidier About page',
    intro: 'Fixes keys that went missing while the Windows key or Alt was held, shows scrolls for each program, and tidies the Contact section. Your existing statistics are kept.',
    items: [
      ['fixed', 'Keys pressed while the Windows key or Alt is held are now counted. Windows handles many of these shortcuts itself (Win+2, Win+T, Alt+Tab and others) and then never passes the key press to the app, only its release. The app now counts that release as the press, so Alt+Tab pressed several times with Alt held counts every Tab. Ctrl and Shift combinations are unchanged.'],
      ['new', 'Scrolls in By application (Overview). Each program now shows keys, clicks and scrolls. Scrolls counted before this version were already stored per program and now show up too.'],
      ['fixed', 'The Contact us section in About was broken: a stray quote mark in its styling made the icons huge and stretched and the Discord bar run across the whole page.'],
      ['improved', 'Contact us is now three matching cards (Instagram, Telegram, Discord) with small fixed-size icons. Discord has a Copy username button that shows Copied! when it worked, or Could not copy when it did not.'],
      ['improved', 'By application rows keep their numbers on one line and wrap neatly on a narrow window.'],
      ['changed', 'Nothing about what is recorded changes: still only key names, mouse buttons, scroll direction and program file names, never typed text or window titles.'],
    ],
  },
  {
    version: '2.2.0', date: '2026-11-18', title: 'Lite mode for older computers, and the logo everywhere',
    intro: 'A Lite mode that lowers processor and memory use, and the chosen logo now also shows in Task Manager and the Start menu. Your statistics are not changed by this version.',
    items: [
      ['new', 'Lite mode (Settings). For older or slower computers. It turns off animations and visual effects at once, and uses a lighter browser engine after the next start of the app: software drawing instead of the graphics card, one page process and no background services. Counting is not affected. A Restart now button finishes the change.'],
      ['improved', 'Less work while the window is hidden. When the window is closed to the tray or minimised, the app now stops animating and redrawing the page. This lowers processor use in normal mode too.'],
      ['fixed', 'The logo you choose now changes everywhere. Before, only the window, the tray icon and the taskbar changed. Task Manager and the Start menu (including pinned tiles) read the icon stored in the program file, so the app now writes your logo into its own file. Windows cannot change a running program, so this is done by a short restart: after you pick a logo, Appearance shows a Restart now button. Your counts are saved first.'],
      ['changed', 'After an update or reinstall the program file has the default icon again. Appearance then shows the Restart now button once so your logo is applied again.'],
    ],
  },
  {
    version: '2.1.0', date: '2026-11-18', title: 'Choose your app logo',
    intro: 'A new choice of app logo. Your statistics are not changed by this version.',
    items: [
      ['new', 'App logo: Appearance now has an App logo section with the 20 logos from the Glassmorphism themes and the alternate light themes. Pick one and it changes the logo in the app and the tray icon and taskbar icon at once. Default restores the original logo.'],
    ],
  },
  {
    version: '2.0.0', date: '2026-11-16', title: 'Tray menu, reminders, summaries, practice and more',
    intro: 'The first major release of KeyClick Tracker. Your existing history is kept: a backup is made before the first start of this version, and the statistics file format is extended without removing anything. Everything below is new in this version unless it is marked as fixed.',
    items: [
      ['new', 'Tray icon: a single left click now opens the app. You no longer need to right-click and choose Open.'],
      ['new', 'Tray menu: it now has a Floating live box switch, so you can turn the live box on or off directly. After a break reminder it also offers Remind me in 5 minutes.'],
      ['improved', 'Tray menu look: the menu has rounded corners and uses the colours of the theme you picked in Appearance.'],
      ['new', 'Active and idle time. A pause longer than the idle threshold (1 to 60 minutes, set in Settings, default 5) is counted as idle time, not work. Overview shows the active and idle time for today. Idle time does not change words per minute or clicks per minute. Time while counting was paused, and the time the PC slept, is not counted as idle.'],
      ['new', 'By application. Overview shows which programs received your keys, clicks and scrolls today. Only the program file name is stored, never window titles or what you type. You can turn it off in Settings; the totals keep counting either way.'],
      ['new', 'Shortcuts. The Keyboard page shows how many key presses were Ctrl, Alt or Windows combinations and what share of your keys they are. Shift alone does not count. Keyboards where AltGr sends Ctrl+Alt are not counted as shortcuts.'],
      ['new', 'Keyboard layouts. The Keyboard page lists the layouts you typed on. Only the layout name is kept. You can turn it off in Settings.'],
      ['new', 'Goals. Add a typing speed goal in Settings. Overview shows a 30-day goal history (one square per day) and your goal streak. Changing a goal never rewrites past days.'],
      ['new', 'Day timeline. Overview shows your activity hour by hour for today, or for any day you pick.'],
      ['new', 'Quiet hours. Choose hours (for example 23:00 to 07:00) when counting is paused. Nothing is deleted, and the status pill shows Quiet hours.'],
      ['new', 'Busy-stretch notes (off by default). A gentle suggestion when your busiest 10 minutes are unusually high compared with your own last 30 days. It needs 14 days of data, it is only a suggestion and never a diagnosis, and it can also appear once a day as a tray note.'],
      ['new', 'Break reminders. Choose a work and rest cycle, not just a single reminder. A reminder never appears during idle time or while a full-screen program is in front, and you can snooze it from the tray menu.'],
      ['new', 'Summaries. Get a short report every evening at 18:00 (today so far) or every Monday at 09:00 (last week, with the change from the week before). It lists totals, active and idle time, your best hour and top programs, and can also be saved as a .md file in Backups\\Reports. You can also make one at any time in Settings. Only counts are used.'],
      ['new', 'Health check on the Data page. It reports whether the statistics file reads correctly, the file size, when it was last saved, the number of backups, the browser cache size, free disk space and whether input counting is running. It only reads; it never changes anything.'],
      ['new', 'Backup preview and restore. On Data and Backup, each backup has a Preview button (totals and date range, nothing changes) and a Restore button. Restore asks you to type RESTORE, saves your current data as a new backup first, and never changes the backup you restore from.'],
      ['new', 'Scheduled backups. One backup a day is kept for as many days as you choose (7 to 90, default 14), and a copy is also made every six hours while the app runs (the latest eight are kept). Other backups are never removed.'],
      ['new', 'Import preview. Before a file is imported, you see what it contains and what would change, and nothing is changed until you confirm.'],
      ['new', 'CSV export of daily totals for spreadsheets. It is an export only: it cannot be imported and is not a backup. Backups and imports stay JSON, TXT and MD.'],
      ['new', 'Displays and start-up. The Data page lists your screens with their scale and shows the start-up steps of this session, which helps when the live box or a hotkey is not where you expect.'],
      ['new', 'Global hotkeys: Ctrl+Alt+K shows or hides the main window, and Ctrl+Alt+L turns the floating live box on or off. They work from any program. Settings shows whether each one is active. If another program already uses a combination, the app says so and keeps running.'],
      ['new', 'Live box styles: compact (one line), detailed, or sparkline (with a 60-second activity line). Each style remembers its own size. Choose in Settings, or right-click the live box to switch to the next style.'],
      ['new', 'Typing practice. A new Practice page with short lessons that score your speed and accuracy. Only the scores are saved: what you type there is never stored.'],
      ['improved', 'Keyboard focus is clearly visible, and animations are reduced when Windows is set to reduce motion.'],
      ['improved', 'Goal values and other settings are kept within a safe range when saved.'],
      ['fixed', 'Typing speed stuck at a number such as 5 or 2 while the PC is idle, in the app and in the floating live box. The speed now returns to 0 after a short pause, even if a screen update was missed.'],
      ['fixed', 'Right Ctrl and right Alt were counted as the left keys. They are now counted separately.'],
      ['fixed', 'The hour-of-day charts and the weekly rhythm were shifted by half an hour for time zones such as India (UTC+5:30). New activity is now counted in the correct hour. Older activity is kept as it is.'],
      ['fixed', 'If saving at exit fails, the reason is now written to the log instead of being ignored.'],
      ['changed', 'The statistics file uses format version 4. The new fields are optional, so older files load unchanged, and a backup is made at the first start of this version. Version 1.x cannot read version 4 files: keep that backup if you ever go back.']
    ]
  },
  {
    version: '1.2.3', date: '2026-11-15', title: 'The live box stays on top and stays on screen',
    intro: 'Closing the window to the system tray no longer makes the floating live box disappear. The live box now stays on top of other windows until you turn it off in Settings or quit the app from the tray.',
    items: [
      ['fixed', 'The floating live box vanished after the main window was closed to the system tray, or after another window was used. Its position was never re-checked, because the check was queued on the wrong WebView2 view, so the box could slip behind other windows. The check now runs, and the box is raised back to the top every 1.5 seconds.'],
      ['changed', 'The live box stays on screen until you turn it off in Settings or quit KeyClick Tracker from the tray menu.'],
    ],
  },
  {
    version: '1.2.2', date: '2026-11-15', title: 'Counts administrator windows, starts at sign-in, now with a setup program',
    intro: 'The real reason keys and clicks were missed in PowerShell and Notepad was found: those windows were running as administrator, and Windows hides administrator windows from normal programs. KeyClick Tracker now runs as administrator too. Your data is backed up before this update runs and is kept.',
    items: [
      ['fixed', 'Keys, clicks and scrolls typed into windows that run as administrator (for example PowerShell or Notepad opened from an administrator PowerShell) were never counted. The app now runs with administrator rights, so it counts input everywhere. Windows will ask for permission when you open the app yourself. At sign-in it starts without a prompt.'],
      ['fixed', 'The app did not start when the computer was turned on. The startup entry pointed at a program file that no longer existed, and a normal startup entry cannot start an administrator program anyway. Start with Windows now uses a sign-in task, and the app repairs the entry every time it opens.'],
      ['new', 'Setup program. KeyClick Tracker now installs to Program Files with a Start Menu shortcut and an entry in Apps and features, so it can be uninstalled cleanly. Your data stays in place when you update or uninstall.'],
      ['fixed', 'Opening a second copy while one is running now simply does nothing instead of starting a second counter that doubled the counts.'],
    ],
  },
  {
    version: '1.2.1', date: '2026-11-15', title: 'Counting in every app, and a steady Little observations list',
    intro: 'Keys, clicks and scrolls are now counted wherever you are working, including PowerShell and Notepad. The Little observations list on Overview no longer blinks while you work. Your data is backed up before this update runs and is kept.',
    items: [
      ['fixed', 'Keys, clicks and scrolls were not counted in some windows, such as PowerShell and Notepad. Input is now read with Windows Raw Input, which receives input from every application whatever has focus, and Windows does not switch it off for being slow. The earlier hook listener is no longer used.'],
      ['changed', 'Input sent by programs (for example automation tools) is now counted too, because Raw Input cannot tell it apart from your own keys and mouse.'],
      ['fixed', 'The Little observations list on Overview no longer blinks and replays its animation on every key, click or scroll. Its rows now update in place and only animate when the set of observations changes.'],
    ],
  },
  {
    version: '1.2.0', date: '2026-11-15', title: 'Reliable counting and a themed live box',
    intro: 'Counting no longer stops silently in some apps, the live box stays visible, and it has its own themes. Your data is backed up before this update runs and is kept.',
    items: [
      ['new', 'Analytics page. An automatic engine studies your counts as they happen and keeps everything up to date live: activity bars from hourly to yearly, a trend line with a moving average, an hourly rhythm with a 24-hour clock, a weekly rhythm, an hour-by-weekday heat map, a year of days, an activity mix and a breakdown of how you type.'],
      ['new', 'Insight engine and profile. Plain-language findings about your habits, such as your pace today against a normal day, your sharpest three hours, consistency, week-on-week momentum, unusual days, typing and correction habits, plus a profile with trait bars. Four live scores (activity score, consistency, momentum and projected total for today) roll their numbers like the other counters. It is ordinary arithmetic on your counts, runs on this computer, and uses no text you type.'],
      ['improved', 'All-time records moved from Overview to the Analytics page.'],
      ['fixed', 'The Insight engine no longer blinks while you work. Its findings are now updated in place instead of being redrawn on every key, click or scroll.'],
      ['fixed', 'While the engine is still learning (fewer than three earlier days), the activity score, consistency and momentum show a dash instead of a misleading number, and findings that need more history wait until there is enough.'],
      ['fixed', 'Keys, clicks and scrolls were sometimes not counted, so speed stayed at 0, mostly in some apps. Windows could silently switch the input listener off when it was slow. Input is now captured on its own thread, never waits for the window, and the listener is re-armed every three seconds.'],
      ['fixed', 'A key that was not counted again after a missed release is now counted when pressed again.'],
      ['fixed', 'The floating live box sometimes vanished. A saved position on a disconnected monitor, a minimized box, or a box pushed behind other windows is now repaired automatically, and the box stays on top.'],
      ['fixed', 'The live box had no colour. It now has its own colour theme.'],
      ['new', 'Floating live box themes: ten calm themes (Ink & Ivory, Parchment, Sage Glass, Dusk Rose, Harbour Blue, Walnut Glow, Linen, Moss Stone, Lavender Dusk, Graphite Mist). Choose one in Appearance; it is separate from the app theme.'],
      ['new', 'Activity glow: the stat boxes light up when they change: keys in blue, clicks in lilac, scrolls in green.'],
      ['new', 'Appearance has an “Official typefaces” section with ten professional families: Times New Roman, Arial, Calibri, Cambria, Georgia, Book Antiqua, Palatino Linotype, Verdana, Tahoma and Comic Sans MS. The other families sit under “Other typefaces”.'],
      ['new', 'Contact us in About: Instagram, Telegram and Discord.'],
      ['improved', 'The ✕ on the live box is smaller and no longer covers the numbers.'],
      ['improved', 'Export and import use only JSON, TXT and MD. Importing any other file type shows a warning and changes nothing. The CSV export was removed.'],
      ['new', 'Real word count. A quiet, hidden engine counts the words you type as you type. It keeps only a counter and a single on/off flag, never the text, and adds almost no CPU or memory. The Key presses card shows the real number. Counting starts with this version, so older history shows no word count.'],
      ['fixed', 'All-time records: the numbers now slide up smoothly when they change, like the other counters, instead of jumping.'],
      ['fixed', 'Choosing a typeface shows its notification in Appearance too, and notifications use your app theme colours.'],
      ['fixed', 'The floating box’s hover glow now follows its rounded corners.'],
      ['fixed', 'The Activity glow now lights up the matching boxes only. The All activity box glows for every key, click and scroll.']
    ]
  },
  {
    version: '1.1.0', date: '2026-11-14', title: 'Live box, levels and a gentler feel',
    intro: 'A floating speed box, a friendlier overview, and a lot more softness in how everything moves.',
    items: [
      ['new', 'Floating live box. A small rounded box with your live speed that stays on top of every window, even while the app is in the tray. Drag it to any corner, pull its bottom-right corner to resize, hover and press ✕ to close. It fades a little when you are not pointing at it and glows back when you are. Turn it on in Settings.'],
      ['new', 'Experience level. A box at the top right shows your level, rank and progress, earned from the events you have made.'],
      ['new', 'Overview: a greeting that follows your computer’s clock (morning, afternoon, evening, night) with a place to type your name, plus a live clock whose digits slide up. Click the clock to switch between 12-hour and 24-hour.'],
      ['new', 'All-time records: most pressed key, most used mouse button, most scrolled direction, busiest day, events made, fastest typing and clicking days, and best minutes.'],
      ['new', 'Measuring units. Show typing speed as WPS, WPM or WPH, and click rate as CPS, CPM or CPH, everywhere including the live box.'],
      ['new', 'Your real KeyClick Tracker logo is now the app, taskbar and tray icon.'],
      ['improved', 'Hover details now appear in a box that matches your theme and font instead of the Windows default tooltip.'],
      ['improved', 'More motion, kept gentle: pages fade and rise, the sidebar marker glides, buttons ripple, switches stretch, cards lift.'],
      ['fixed', 'The WPM and CPM at the top of the window always showed 0. They now update live.'],
      ['fixed', 'Rolling numbers looked ragged in fonts such as Georgia. They now stay aligned in every font.']
    ]
  },
  {
    version: '1.0.0', date: '2026-11-14', title: 'KeyClick Tracker',
    intro: 'KeyScope became KeyClick Tracker, with a calmer look and numbers that move the moment you do.',
    items: [
      ['new', 'Live numbers: key presses, clicks and scrolls climb as you press, click and scroll, with sliding digits.'],
      ['new', 'Typing speed (WPM) and click rate (CPM): live, overall, average and peak, plus a session view and a short typing test.'],
      ['new', 'A macOS-style window with rounded corners and traffic-light buttons on the right.'],
      ['new', '30 hand-tuned colour themes and 15 font families, applied instantly and remembered.'],
      ['new', 'Import and export in JSON, TXT and Markdown, with CSV for spreadsheets. Imports merge safely and never double-count.'],
      ['new', 'Daily goals, streaks, a weekly rhythm heatmap, milestones, break reminders, an update log and start-with-Windows.'],
      ['improved', 'Your history is protected on every update: a backup is made before the first run of a new version, plus a daily snapshot.'],
      ['fixed', '“Yesterday” and the “Previous …” ranges wrongly included everything up to now.'],
      ['fixed', 'A damaged data file is no longer silently replaced with an empty one.'],
      ['fixed', 'Starting the app twice no longer counts every key twice.']
    ]
  },
  {
    version: 'KeyScope', date: 'Before KeyClick Tracker', title: 'KeyScope',
    intro: 'The original desktop release: private counting of key presses, mouse buttons and scrolling.',
    items: [
      ['new', 'Counted keys, mouse buttons and scroll direction without storing what you type.'],
      ['new', 'System tray icon with pause and resume.']
    ]
  }
];
