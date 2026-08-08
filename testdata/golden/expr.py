a, b, c = 3, 2, 1
x = (a + b) * c
y = - a**2
z = not a == b
w = a < b < c
p = a if b > c else b
q = a + b if a > c and b < a else a - b
s = a * b + c / a - (a - b)**2
t = ~ a & b | c ^ a
u = a << 2 >> 1
v = a // b % c
lst = [x, y, z, w, p, q, s, t, u, v]
print(lst)
print([(i, j) for i in range(3) for j in range(3) if i + j == 2])
print({1, 2, 2, 3}, {})
print((a,), a, a)
print(1_000_000, 0xFF, 0b1010, 0o17, 1.5e3, 1.5, .5)
print('单引号', "双引号", '转义\'引号', "a\tb", "多行\
拼接")
print("""三引号
跨行""")
