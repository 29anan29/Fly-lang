"""Fly 安全输入验证示例。"""

class GuardError(Exception):
    """Fly guard 断言失败"""

    pass

def create_user(username, age):
    if not (isinstance(username, str) and (len(username) > 0) and (len(username) <= 20)):
        raise GuardError("guard username: str, len(username) > 0, len(username) <= 20")
    if not (isinstance(age, int) and (0 < age < 150)):
        raise GuardError("guard age: int, 0 < age < 150")
    return username + str(age)
def ratio(b):
    if not ((b != 0)):
        raise GuardError("guard b != 0")
    return 1 / b
def is_admin(flag):
    if not ((flag)):
        raise GuardError("guard flag")
    return flag
