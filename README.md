# P1Mon imitator
This repository contains a single Go Binary which imitates the p1mon ( https://www.p1-monitor.nl/ ) API.

The single Go binary can be run on a Linux machine where the p1 port of a Dutch Smart Energy meter is connected via USB and present, often as /dev/ttyUSB0

This binary will read the incoming values from the p1mon, store them in memory and serve them via the API.

# AMD64 and ARCH64
The binary can be compiled as AMD64 and ARCH64, where for the last one it can run on a Raspberry Pi or similar device.

# Electricity only
The only supported functionality is electricity, water and gas are not supported at this moment.

# Home Assistant
The goal is that Home Assistant can read the API via this integrtion: https://www.home-assistant.io/integrations/p1_monitor/

The integration should not notice that its an imitator that pretents to be p1mon, but is not.
