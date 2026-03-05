  cd ~/projects/github/lootek/yt-daily-player
  yt-dlp --cookies-from-browser safari --cookies cookies.txt --skip-download "https://youtube.com"
  scp cookies.txt pi@192.168.10.22:~/yt-daily-player/
  ssh pi@192.168.10.22 'cd ~/yt-daily-player && docker compose restart'
