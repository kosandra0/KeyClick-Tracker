/* Update log shown in the app. Newest first. Add a new entry on every release. */
window.KC_CHANGELOG = [
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
