#!/bin/sh

exec ./nexus_proxy \
  "--upstream_url=smooth=http://vps4.functor.xyz/smooth/" \
  "--prefetch=smooth=generic=http://vps4.functor.xyz/smooth/" \
  "--gc_max_age=smooth=2000h"
