enter plan mode.

first, figure out what the service in ~/projects/lootek/yt-player-scheduler does.
mind, it is deployed at pi@ithilien in ~/yt-daily-player/ (feel free to ssh into there) 

now, your task is to extend the functionality by adding a web UI where a user can paste a link to the YT video, channel or playlist and it'll all be downloaded to the directory that is to be provided in the configuration (not necessarily scheduled for playing through mpd - let's have a checkbox for that)

few more notes to consider for the plan:
1. for storing downloaded content, on the "ithilien" remote box, there's a dedicated directory already /media/music/youtube
2. for options of downloading files and for naming pattern, be consistent with the legacy ~/scripts/yt.sh
3. add another checkbox for the user to decide whether do dump video or not (ie. music only)

ask me for anything you're unsure about.

When done, copy the resulting plan to an .md file named after your model to ~/ai/yt-player-scheduler/
