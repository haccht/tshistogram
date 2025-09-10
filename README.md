# tshistogram

Render time series data as a histogram in the terminal.


```
$ tshistogram -h
Usage:
  tshistogram [Options] [file...]

Options:
  -f, --format string       Input time format (default: auto)
  -i, --interval duration   Bin width as duration (e.g. 30s, 1m, 1h) (default 5m0s)
  -b, --barlength int       Length of the longest bar (default 60)
  -l, --location location   Timezone location (e.g., UTC, Asia/Tokyo) (default Local)
      --color string        Markup the bar: 'never', 'always', 'auto' (default "auto")

Format Examples:
  ANSIC       "Mon Jan _2 15:04:05 2006"
  UnixDate    "Mon Jan _2 15:04:05 MST 2006"
  RubyDate    "Mon Jan 02 15:04:05 -0700 2006"
  RFC822      "02 Jan 06 15:04 MST"
  RFC822Z     "02 Jan 06 15:04 -0700"
  RFC850      "Monday, 02-Jan-06 15:04:05 MST"
  RFC1123     "Mon, 02 Jan 2006 15:04:05 MST"
  RFC1123Z    "Mon, 02 Jan 2006 15:04:05 -0700"
  RFC3339     "2006-01-02T15:04:05Z07:00"
  RFC3339Nano "2006-01-02T15:04:05.999999999Z07:00"
  Kitchen     "3:04PM"
  Stamp       "Jan _2 15:04:05"
  StampMilli  "Jan _2 15:04:05.000"
  StampMicro  "Jan _2 15:04:05.000000"
  StampNano   "Jan _2 15:04:05.000000000"
  DateTime    "2006-01-02 15:04:05"
  DateOnly    "2006-01-02"
  TimeOnly    "15:04:05"
  Unix        "1136239445"
  Unix-Milli  "1136239445000"
  Unix-Micro  "1136239445000000"

  Arbitrary formats are also supported. See https://pkg.go.dev/time as a reference.
```

`tshistogram` render histograms from the given list of time series list.

```
$ cat /var/log/syslog | tail -10
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' suspended (module 'builtin:omfile'), retry 0. There should be messages before this one giving the reason for suspension. [v8.2112.0 try https://www.rsyslog.com/e/2007 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' resumed (module 'builtin:omfile') [v8.2112.0 try https://www.rsyslog.com/e/2359 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' suspended (module 'builtin:omfile'), retry 0. There should be messages before this one giving the reason for suspension. [v8.2112.0 try https://www.rsyslog.com/e/2007 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' resumed (module 'builtin:omfile') [v8.2112.0 try https://www.rsyslog.com/e/2359 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' suspended (module 'builtin:omfile'), retry 0. There should be messages before this one giving the reason for suspension. [v8.2112.0 try https://www.rsyslog.com/e/2007 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' resumed (module 'builtin:omfile') [v8.2112.0 try https://www.rsyslog.com/e/2359 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' suspended (module 'builtin:omfile'), retry 0. There should be messages before this one giving the reason for suspension. [v8.2112.0 try https://www.rsyslog.com/e/2007 ]
Nov  9 05:44:54 localhost rsyslogd: action 'action-0-builtin:omfile' suspended (module 'builtin:omfile'), next retry is Thu Nov  9 05:45:24 2023, retry nbr 0. There should be messages before this one giving the reason for suspension. [v8.2112.0 try https://www.rsyslog.com/e/2007 ]
Nov  9 05:45:01 localhost CRON[1026406]: (root) CMD (command -v debian-sa1 > /dev/null && debian-sa1 1 1)
Nov  9 05:45:12 localhost kernel: [2408786.637431] [UFW BLOCK] IN=eth0 OUT= MAC=f2:3c:93:6e:f2:1c:fe:ff:ff:ff:ff:ff:08:00 SRC=94.102.61.28 DST=172.233.65.222 LEN=40 TOS=0x00 PREC=0x00 TTL=238 ID=54321 PROTO=TCP SPT=35385 DPT=40933 WINDOW=65535 RES=0x00 SYN URGP=0 

$ cat /var/log/syslog | tail -10 | cut -c1-15
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:44:54
Nov  9 05:45:01
Nov  9 05:45:12

$ cat /var/log/syslog | tail -10000 | cut -c1-15 | tshistogram -i 15m -f stamp --time-zone Asia/Tokyo
Total count = 10000
Time range  = 2023-11-09T09:55:33+09:00 - 2023-11-09T15:01:18+09:00

 [ 2023-11-09T09:45:00+09:00 ]    164  ||||||||||||
 [ 2023-11-09T10:00:00+09:00 ]    448  ||||||||||||||||||||||||||||||||||
 [ 2023-11-09T10:15:00+09:00 ]    475  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T10:30:00+09:00 ]    471  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T10:45:00+09:00 ]    494  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T11:00:00+09:00 ]    468  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T11:15:00+09:00 ]    517  ||||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T11:30:00+09:00 ]    515  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T11:45:00+09:00 ]    452  ||||||||||||||||||||||||||||||||||
 [ 2023-11-09T12:00:00+09:00 ]    516  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T12:15:00+09:00 ]    473  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T12:30:00+09:00 ]    471  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T12:45:00+09:00 ]    516  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T13:00:00+09:00 ]    472  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T13:15:00+09:00 ]    496  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T13:30:00+09:00 ]    492  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T13:45:00+09:00 ]    516  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T14:00:00+09:00 ]    510  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T14:15:00+09:00 ]    498  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T14:30:00+09:00 ]    516  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T14:45:00+09:00 ]    473  ||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T15:00:00+09:00 ]     47  |||


$ cat /var/log/syslog | cut -c1-15 | tshistogram -i 6h -f stamp --time-zone Asia/Tokyo
Total count = 202378
Time range  = 2023-11-05T09:19:13+09:00 - 2023-11-09T15:07:59+09:00

 [ 2023-11-05T09:00:00+09:00 ]  11600  |||||||||||||||||||||||||||||||||||||
 [ 2023-11-05T15:00:00+09:00 ]  12083  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-05T21:00:00+09:00 ]  12100  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-06T03:00:00+09:00 ]  12335  ||||||||||||||||||||||||||||||||||||||||
 [ 2023-11-06T09:00:00+09:00 ]  12148  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-06T15:00:00+09:00 ]  12010  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-06T21:00:00+09:00 ]  11928  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-07T03:00:00+09:00 ]  12286  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-07T09:00:00+09:00 ]  12100  |||||||||||||||||||||||||||||||||||||||
 [ 2023-11-07T15:00:00+09:00 ]  11914  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-07T21:00:00+09:00 ]  11789  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-08T03:00:00+09:00 ]  11536  |||||||||||||||||||||||||||||||||||||
 [ 2023-11-08T09:00:00+09:00 ]  11619  |||||||||||||||||||||||||||||||||||||
 [ 2023-11-08T15:00:00+09:00 ]  11626  |||||||||||||||||||||||||||||||||||||
 [ 2023-11-08T21:00:00+09:00 ]  11507  |||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T03:00:00+09:00 ]  11764  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T09:00:00+09:00 ]  11755  ||||||||||||||||||||||||||||||||||||||
 [ 2023-11-09T15:00:00+09:00 ]    278  
```
