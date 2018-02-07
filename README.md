gophertrak
==========

A colorful console-based tracker for APRS-enabled high altitude balloons.   

![GopherTrak](https://chrissnell.com/images/github/gophertrak.png)


What Works
----------
* APRS packet receiption via TNC using my [tnc-server](http://github.com/chrissnell/tnc-server) software
* APRS packet decoding with [GoBalloon](http://github.com/chrissnell/GoBalloon)'s APRS library
* GPS position receiption via gpsd
* Text-based UI via termbox-go and my drawing primitives

In Progress
-----------
* Improved error handling for TNC connections
* Real-time packet log

Not Yet Started
---------------
* Chasers display - distance/direction to other balloon chasers and the balloon
* Configuration via YAML config file


