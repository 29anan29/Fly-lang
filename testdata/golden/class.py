class Animal:
    legs = 4
    def __init__(self, name):
        self.name = name
    def speak(self):
        return f"{self.name} makes a sound"
a = Animal("dog")
print(a.legs, a.speak())
