import syslog
print(syslog.LOG_EMERG, syslog.LOG_ERR, syslog.LOG_INFO, syslog.LOG_DEBUG, syslog.LOG_USER, syslog.LOG_LOCAL0, syslog.LOG_DAEMON, syslog.LOG_PID, syslog.LOG_CONS, syslog.LOG_NDELAY, syslog.LOG_PERROR)
print(syslog.LOG_MASK(syslog.LOG_ERR), syslog.LOG_UPTO(syslog.LOG_ERR))
syslog.openlog("gpython-test", syslog.LOG_PID, syslog.LOG_USER)
old = syslog.setlogmask(syslog.LOG_UPTO(syslog.LOG_ERR))
print(old, syslog.setlogmask(0))
syslog.syslog("gpython syslog test")
syslog.syslog(syslog.LOG_ERR, "gpython syslog test err")
syslog.closelog()
for f in (lambda: syslog.syslog(), lambda: syslog.syslog(1), lambda: syslog.openlog(5), lambda: syslog.syslog("a\0b")):
    try: f()
    except Exception as e: print(type(e).__name__)
