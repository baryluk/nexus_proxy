#!/bin/bash

./nexus_proxy \
  --upstream_url=debian=http://deb.debian.org/debian/ \
  --upstream_url=debian-debug=http://deb.debian.org/debian-debug/ \
  --prefetch=debian=generic=https://mirror.init7.net/debian/ \
  --gc_max_age=debian=1440h \
  --gc_max_age=debian-debug=24h \
  '--prefetch_exclude=debian=.*(experimental|(old)+stable|proposed-updates|rc-buggy|jessie|buster|bookworm|bullseye|stretch|Debian([2-9]|10|11)).*|.*(_|-)(armel|arm64|mips64el|mipsel|i386|s390x|ppc64el|armhf|mips|source).*|installer-amd64|.*\.debian\.tar\.(xz|gz)(\.asc)?$|\.dsc$|\.orig.tar.(xz|gz|bz2)(\.asc)?$|\.diff\.gz$'


#2022/08/09 23:30:24 prefetcher: Matched 118759 "https://mirror.init7.net/debian/ls-lR.gz"
#2022/08/09 23:30:24 prefetcher: Update loop finished in 2m14.158015469s

#2022/08/13 01:14:37 prefetcher: Matched 118571 "https://mirror.init7.net/debian/ls-lR.gz"
#2022/08/13 01:14:37 prefetcher: Update loop finished in 2m21.928649133s

# Even if output is redirected to not print in terminal, it still take same time:
# 2022/08/13 01:18:45 prefetcher: Update loop finished in 2m23.132163134s
