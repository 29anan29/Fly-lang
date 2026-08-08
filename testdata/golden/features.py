import json
from math import sqrt as s, pi
def process(items):
    total = 0
    for x in items:
        total += x * 2
    while total < 100:
        total *= 2
        if total > 500:
            break
    else:
        print("never")
    try:
        data = json.loads('{"a": [1, 2, 3]}')
        return data["a"][1:3], total
    except (ValueError, TypeError) as e:
        print(f"err: {e}")
    finally:
        print("done")
a, b = process([1, 2, 3])
print(a, b)
sq = [i * i for i in range(10) if i % 2 == 0]
d = {"x": 1, "y": s(9)}
print(sq, d, pi)
