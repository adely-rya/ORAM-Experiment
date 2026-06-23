import matplotlib.pyplot as plt

read_ratio = [50, 70, 90, 99,99.9]
tvd = [0.2621811759, 0.1671631065, 0.1246631065, 0.0521811759,0.0177852543]
avg_active_sig = [1.6665, 2.4721, 4.8960, 8.5397,8.1572]

mvp_tvd = 0.1421811759

plt.figure(figsize=(7, 4))
plt.plot(read_ratio, tvd, marker="o", label="Re-MVP-ORAM")
plt.axhline(y=mvp_tvd, linestyle="--", label="MVP-ORAM baseline")

plt.xlabel("Read ratio (%)")
plt.ylabel("Total variation distance")
plt.title("TVD vs Read Ratio")
plt.grid(True)
plt.legend()
plt.tight_layout()
plt.show()


plt.figure(figsize=(7, 4))
plt.plot(avg_active_sig, tvd, marker="o")

plt.xlabel("Average active signatures")
plt.ylabel("Total variation distance")
plt.title("TVD vs Active Signatures")
plt.grid(True)
plt.tight_layout()
plt.show()