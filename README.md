# btu

`btu` converts Bambu Studio 3MF material assignments into a Snapmaker U1 3MF
while preserving the source model geometry. It supports both ordinary
geometry-layered projects and Bambu native Full Spectrum mixed materials. For
ordinary projects, colors that cannot use a physical filament slot are
automatically converted into U1 Full Spectrum mixtures.

The built-in U1 baselines supply printer, process, and physical filament
profiles for 0.2, 0.4, 0.6, and 0.8 mm nozzles. The source supplies geometry,
layer height, material references, face painting, and supported mixed-material
definitions. An optional U1 3MF can override the built-in baselines.

## Build

```sh
go build -o btu ./cmd/btu
```

## Convert

```sh
./btu \
    ~/Downloads/Fantasmino.3mf
```

Without `--output`, this writes `~/Downloads/Fantasmino-btu.3mf`. When that
file already exists, an interactive terminal asks before replacing it. Pass
`--replace` for non-interactive replacement or to skip the prompt.

Options:

| Long | Short | Required | Purpose |
| --- | --- | --- | --- |
| `--output FILE` | `-o FILE` | no | Override the default `SOURCE-btu.3mf` path |
| `--replace` | `-r` | no | Replace an existing output without prompting |
| `--colors ORDER` | `-c ORDER` | no | Set the project-preview palette to `cmyg`, `cmyw`, or `cmyb` |
| `--full-spectrum` | `-f` | no | Synthesize source colors from the loaded filaments |
| `--nozzle DIAMETER_MM` | `-n DIAMETER_MM` | no | Force a built-in baseline; accepts `0.2`, `0.4`, `0.6`, or `0.8` (millimeters) |
| `--template FILE` | `-t FILE` | no | Override the built-in U1 baseline |

By default, `btu` reads `nozzle_diameter` from the source 3MF and selects the
matching built-in U1 baseline. Override that selection when the U1 will use a
different nozzle:

```sh
./btu \
    --nozzle 0.6 \
    ~/Downloads/model.3mf
```

The selected baseline supplies the Snapmaker U1 printer, process, nozzle,
filament, and slice metadata. Pass `--template` only when a different U1 3MF
should override the built-in baseline. When `--template` and `--nozzle` are
used together, the template is loaded first and the selected built-in nozzle
profile is applied on top; unrelated template settings and metadata remain.

Slots 1 through 3 are always cyan, magenta, and yellow. Slot 4 is the neutral
slot and can contain gray, white, or black:

- `c`: cyan/blue
- `m`: magenta/red
- `y`: yellow
- `g`: gray
- `w`: white
- `b`: black

The default project-preview sequence is `cmyg`. Use `cmyw` or `cmyb` only when
white or black should be the neutral shown by Orca for the whole project:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --colors cmyb \
    ~/Downloads/model.3mf
```

For ordinary multi-plate projects, `btu` reads each plate's `plater_id`,
`plater_name`, object membership, and actual material usage. It keeps CMY fixed
and independently chooses gray, white, or black for T4 on each non-empty plate.
The output remains one multi-plate 3MF with one global material mapping. The CLI
groups plates that use the same T4 filament and prints a recommended order,
using each plate's sequence number and plate name. The current preview T4 group
is printed first, and T4 is replaced only between groups, minimizing filament
changes without modifying the plate order stored in the 3MF.

Orca cannot represent gray, white, and black as separate one-component virtual
materials backed by the same physical slot. The project therefore keeps the
`--colors` neutral for preview (`cmyg` by default), while the printed result uses
the T4 filament named in the per-plate steps. An empty plate name remains empty;
`btu` does not substitute a model name.

The converter maps the neutral selected for a plate directly to T4 and mixes
the other mapped colors when needed. It searches one- to four-component recipes
with integer percentages against that plate's actual `cmyg`, `cmyw`, or `cmyb`
filaments,
scores their preview colors with the same MIT-licensed FilamentMixer model used
by FullSpectrum, creates the required virtual materials, remaps model and
face-paint references, and enables U1 Local-Z mixed-color printing. If the same
source material is printed on plates that choose different T4 colors, its shared
recipe uses CMY only so that one material ID remains valid across the project.
Declared but unused colors retain deterministic mappings without influencing a
plate's T4 choice. Preview colors remain estimates because physical filament
opacity and transmission vary.

For example, a four-plate project may report:

```text
U1 colors: cmyg
Printing steps (recommended order, 2 T4 changes):
  keep T4 gray (cmyg): plate 4 - nose
  replace T4 gray -> white (cmyw): plate 1 - eyes, plate 3 - ears
  replace T4 white -> black (cmyb): plate 2 - body
```

```sh
./btu \
    --output ~/Downloads/model-full-spectrum.3mf \
    --full-spectrum \
    ~/Downloads/model.3mf
```

When any declared source color is not one of the six available colors and
`--full-spectrum` was not supplied, an interactive terminal asks whether to
generate the colors with full-spectrum mixing. Answering yes keeps the existing
four-filament synthesis behavior. Answering no creates one ordinary material
slot for every source material, keeps the source T1...Tn order and per-material
settings, and creates no virtual mixtures. This non-mixed result is not limited
to four logical material slots. Non-interactive runs return an error with
instructions to pass `--full-spectrum`.

Override the built-in baseline when needed:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --template ~/Downloads/custom-u1.3mf \
    ~/Downloads/model.3mf
```

Progress uses Bubble Tea in an interactive terminal and plain stage messages
when output is redirected or run without a TTY.

Replacement is published only after the temporary 3MF passes verification.
Declining the prompt leaves the existing output unchanged. Bambu
penetration-layer settings have no direct U1 equivalent and are not transferred.
Non-linear or per-part gradient curves are rejected rather than approximated. A
project that already contains native mixed materials uses its existing
definitions and must not be passed with `--full-spectrum`.
