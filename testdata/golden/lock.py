SECRET_KEY = "abc-123"
x = 42
def read():
    return SECRET_KEY + str(x)
print(read())
