  cd ~/projects/github/lootek/yt-daily-player
  yt-dlp --cookies-from-browser safari --cookies cookies.txt --skip-download "https://youtube.com"
  scp cookies.txt ${RPI_USER}@${RPI_IP}:~/yt-daily-player/
  ssh ${RPI_USER}@${RPI_IP} 'cd ~/yt-daily-player && docker compose restart'
