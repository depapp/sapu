# Sapu: bersihkan HP Android kamu dari terminal atau dari aplikasi, gratis dan open source

*Draf posting peluncuran. Versi pendek untuk X/Threads/Twitter di bagian bawah.*

Kalau kamu pernah kagum sama [Mole](https://github.com/tw93/mole), "CleanMyMac gratis" yang viral di GitHub, ini versi Android-nya: **Sapu**.

Sapu itu bahasa Indonesia untuk broom. Sapu menyapu HP Android kamu sampai bersih: sisa folder dari aplikasi yang sudah dihapus, APK installer lama, unduhan yang tidak selesai, cache thumbnail, cache semua aplikasi, sampai bloatware bawaan Samsung, Xiaomi, OPPO, vivo, Infinix dan Tecno yang tidak bisa kamu copot dari Pengaturan. Tanpa root, tanpa iklan, tanpa "upgrade ke premium".

## Kenapa butuh ini

Sejak Android 11, aplikasi pembersih di Play Store sebenarnya tidak bisa berbuat banyak. Mereka tidak boleh membaca folder `Android/data` aplikasi lain, tidak boleh menghapus cache aplikasi lain, dan tidak boleh mencopot aplikasi bawaan. Jadi yang mereka lakukan kebanyakan animasi roket plus iklan.

Yang bisa melakukan semua itu adalah **adb**, jembatan debugging yang dipakai developer Android. Sapu memakainya: dari komputer lewat kabel USB, atau dari HP itu sendiri lewat [Shizuku](https://shizuku.rikka.app) (sekali pasang, tanpa root).

## Dua cara pakai

**CLI** (macOS, Linux, Windows), satu binary, tampilan terminal yang enak dilihat:

```bash
curl -fsSL https://raw.githubusercontent.com/depapp/sapu/main/install.sh | sh
sapu            # menu interaktif
sapu clean      # bersihkan
sapu uninstall --bloat   # pilih bloatware yang mau dicopot
sapu analyze    # lihat apa yang makan storage
sapu status     # baterai, suhu, RAM, CPU, jaringan
sapu optimize   # kompilasi aplikasi favorit, dexopt, audit baterai
```

**Aplikasi Android** (APK, Kotlin + Jetpack Compose, bahasa Indonesia): tab Beranda, Bersihkan, Aplikasi, Optimalkan dan Sampah. Butuh Shizuku untuk menyapu dan mencopot; tanpa Shizuku tetap bisa melihat storage dan daftar aplikasi.

Keduanya membaca basis data rules yang sama dan menulis ke folder sampah yang sama, jadi apa yang disapu dari aplikasi bisa dipulihkan dari terminal, dan sebaliknya.

## Aman dulu, baru bersih

Ini bagian yang paling saya pikirkan, karena menghapus file orang itu urusan serius:

- **Pratinjau dulu.** Setiap perintah punya `--dry-run` (di aplikasi: tombol Pratinjau). Kamu lihat persis apa yang akan terjadi sebelum ada yang berubah.
- **Semua masuk sampah.** File tidak dihapus, tapi dipindah ke `/sdcard/.sapu/trash` lengkap dengan manifest. `sapu trash restore` mengembalikan semuanya. Satu-satunya langkah permanen adalah pembersihan cache bawaan Android (sama seperti Pengaturan > Penyimpanan > Hapus cache), dan itu diberi label jelas.
- **Aplikasi bawaan tidak hilang beneran.** Dicopot untuk pengguna saat ini saja; `sapu restore` mengembalikannya kapan saja.
- **Data pribadi dilindungi.** Media WhatsApp, Telegram, aplikasi bank, e-wallet, authenticator dan password manager tidak pernah disentuh, bahkan saat kamu mencopot aplikasinya.
- **Daftar lindung.** Dialer, SMS, Play Services, keyboard, launcher dan komponen sistem lain ditolak, apa pun yang kamu ketik.
- **Satu pintu penghapusan.** Semua operasi destruktif lewat satu fungsi dengan validasi path, dan CI menolak kode yang memanggil `rm` atau `pm uninstall` di tempat lain.
- **Riwayat, tanpa telemetri.** `sapu history` mencatat semua yang terjadi. Tidak ada data yang dikirim ke mana pun.

## Bantu rules-nya tumbuh

Basis data bloatware masih muda dan baru diuji di emulator plus daftar publik. Kalau kamu punya HP Samsung, Xiaomi, OPPO, vivo, Infinix atau Tecno, jalankan `sapu probe` dan tempel hasilnya ke issue: isinya hanya model HP, versi Android dan paket bawaan yang belum dikenal, tanpa nomor seri, akun, nama aplikasi pribadi atau nama file.

Repo: https://github.com/depapp/sapu (GPL-3.0). Bintangnya membantu orang lain menemukan ini.

---

## Versi pendek

Bikin "Mole" untuk Android: **Sapu**, pembersih dan debloater open source, tanpa root.

Dari terminal (`sapu clean`, `sapu uninstall --bloat`, `sapu analyze`, `sapu status`, `sapu optimize`) atau dari aplikasi Android berbahasa Indonesia lewat Shizuku.

Semua ada pratinjau, semua masuk sampah dan bisa dipulihkan, data WhatsApp dan aplikasi bank tidak disentuh, tanpa iklan dan tanpa telemetri.

https://github.com/depapp/sapu
