# beam_load

Reactions, shear, bending moment and deflection of statically determinate beams.

Beams: `simply_supported` (a pin and a roller at 0 and L, or at `supports: [a, b]` for one or two
overhangs) and `cantilever` (fixed at x = 0, free at x = L; mirror the positions for a beam fixed
on the right). Loads act downwards: `point` (force at a position), `udl` (uniform force per
length, whole span or `start`–`end`) and `linear` (intensity varying linearly from
`start_magnitude` to `end_magnitude`, such as a triangular load).

Method. Reactions come from equilibrium. The beam is cut at every load edge and support, so the
load is linear on each piece; the shear V(x) = V(x₀) − ∫q is then a polynomial of degree ≤ 2
and the moment M(x) = M(x₀) + ∫V of degree ≤ 3, carried exactly in fractions. With E and I the
elastic curve EI·y″ = M is integrated twice, exactly, and the two constants come from the
supports (y = 0 at both supports, or y = y′ = 0 at the fixed end). Extremes inside a piece (zero
shear for the moment, zero slope for the deflection) are found by bisection to 50 digits;
values at the supports, loads and given `points` are exact. Points under a point load show
`shear_left` and `shear_right`.

Signs: x from the left end; loads down positive; reactions up positive; sagging moment positive;
deflection reported downward positive (an overhang tip that rises is negative); for a cantilever
the reaction couple is given counter-clockwise positive.

Units: `SI` (N, m, N/m, Pa, m⁴; moments in N·m, deflections in m), `imperial` (lbf, in, lbf/in,
psi, in⁴; lbf·in, in) or `consistent` (any coherent set). The units used are echoed in the
output.

When the beam carries one standard load, `table_check` gives the closed form of the beam tables
(AISC Steel Construction Manual Table 3-23, Roark Table 8.1): simple beam with uniform load
(wL²/8, 5wL⁴/384EI), centre or off-centre point load (PL/4, PL³/48EI; Pab/L), load increasing
to one end (R = W/3 and 2W/3, 2WL/(9√3), 0.01304 WL³/EI); cantilever with uniform load
(wL²/2, wL⁴/8EI) or point load (PL, PL³/3EI; Pa²(3L − a)/6EI).

The tests compare the integrated results with those formulas worked by hand: a 10 kN/m, 6 m
simple beam (45 kN·m, 10.546875 mm), a cantilever end load (PL³/3EI), a centre load, the
triangular load, an imperial case, superposition on a cantilever and an overhanging beam.

Every answer carries this disclaimer: elastic small-deflection theory for a prismatic,
statically determinate beam, not a structural design check (no strength, stability, shear
deformation, load factors or code compliance); have structural work checked by a qualified
engineer.

Limits: no applied couples, no continuous or fixed-fixed (indeterminate) beams, no varying
section, no axial load or shear deformation.
