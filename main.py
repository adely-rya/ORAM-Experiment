import math
import matplotlib.pyplot as plt

x1 = list(range(30, 35))
x2 = list(range(96,128))

fact = [8**i for i in x1]
exp2 = [2**i for i in x2]

plt.figure()
plt.plot(x1, fact, marker='o', label='n!')
plt.plot(x2, exp2, marker='o', label='2^n')
plt.xlabel('n')
plt.ylabel('value')
plt.title('Factorial vs Exponential (2^n)')
plt.legend()
plt.grid()

plt.show()
