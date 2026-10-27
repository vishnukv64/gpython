import statistics as s
from fractions import Fraction
d = [2, 4, 4, 4, 5, 5, 7, 9]
print(s.mean(d), s.median(d), s.stdev(d), s.pstdev(d), s.variance(d), s.pvariance(d))
print(s.mean([1.5, 2.5, 3.5]), s.fmean([1, 2, 3, 4]), s.median_low([1, 3, 5, 7]), s.median_high([1, 3, 5, 7]))
print(s.mode([1, 1, 2, 3]), s.multimode([1, 1, 2, 2]), s.geometric_mean([1, 2, 4]), s.harmonic_mean([1, 2, 4]))
print(s.mean([Fraction(1, 3), Fraction(2, 3)]), s.quantiles(d, n=4), s.median_grouped([1, 2, 2, 3, 4]))
print(s.correlation([1, 2, 3, 4], [1, 2, 3, 5]), s.linear_regression([1, 2, 3], [2, 4, 6]))
try:
    s.mean([])
except s.StatisticsError as e:
    print("StatisticsError", e)
print(s.NormalDist(0, 1).cdf(1.0), s.NormalDist(10, 2).pdf(10))
