import functools as _fly_functools
import resource as _fly_resource
import signal as _fly_signal


class ResourceExhaustedError(RuntimeError):
    """Fly cage 资源超限"""


def _fly_timeout_handler(signum, frame):
    raise TimeoutError("cage: 执行超时")


def _fly_cage(max_time=None, max_memory=None):
    def deco(fn):
        @_fly_functools.wraps(fn)
        def wrapped(*args, **kwargs):
            prev_alarm = None
            prev_rlimit = None
            try:
                if max_time is not None:
                    prev_alarm = _fly_signal.getsignal(_fly_signal.SIGALRM)
                    _fly_signal.signal(_fly_signal.SIGALRM, _fly_timeout_handler)
                    _fly_signal.setitimer(_fly_signal.ITIMER_REAL, max_time)
                if max_memory is not None:
                    prev_rlimit = _fly_resource.getrlimit(_fly_resource.RLIMIT_AS)
                    soft, hard = prev_rlimit
                    if soft == _fly_resource.RLIM_INFINITY or max_memory < soft:
                        soft = max_memory
                    _fly_resource.setrlimit(_fly_resource.RLIMIT_AS, (soft, hard))
                try:
                    return fn(*args, **kwargs)
                except MemoryError:
                    raise ResourceExhaustedError(
                        "cage: 内存超限（限制 %d 字节）" % max_memory
                    )
            finally:
                if max_time is not None:
                    _fly_signal.setitimer(_fly_signal.ITIMER_REAL, 0)
                    _fly_signal.signal(_fly_signal.SIGALRM, prev_alarm)
                if max_memory is not None:
                    _fly_resource.setrlimit(_fly_resource.RLIMIT_AS, prev_rlimit)

        return wrapped

    return deco

import time
@_fly_cage(2, 52428800)
def heavy():
    data = [0] * 100_000_000
    return len(data)
@_fly_cage(2, 52428800)
def busy():
    while True:
        time.sleep(0.01)
@_fly_cage(0.5)
def quick():
    return 1
print("cage-ready")
