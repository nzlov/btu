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
| `--colors CMYGWB` | `-c CMYGWB` | no | Preferred base-color order for slots 1 through 4 |
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

Use four characters to describe the preferred colors in slots 1 through 4:

- `c`: cyan/blue
- `m`: magenta/red
- `y`: yellow
- `g`: gray
- `w`: white
- `b`: black

The default sequence is cyan, magenta, yellow, and gray: `cmyg`. Each color can
appear at most once. For example, black, magenta, yellow, and cyan in slots 1
through 4 is `bmyc`:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --colors bmyc \
    ~/Downloads/model.3mf
```

For ordinary projects, `btu` inspects every declared material color, including
colors that currently have no model faces. It recognizes cyan, magenta, yellow,
gray, white, and black, then chooses four physical filaments. Every declared
color constrains that choice and receives either its corresponding physical slot
or a generated virtual mixture. CMYG remains the preferred order when candidate
palettes reproduce the declared colors equally well. The final order is printed
as `U1 colors`; load those reported colors before printing.

The converter mixes only when a mapped color cannot use a physical slot. It
searches one- to four-component recipes with integer percentages,
scores their preview colors with the same MIT-licensed FilamentMixer model used
by FullSpectrum, creates the required virtual materials, remaps model and
face-paint references, and enables U1 Local-Z mixed-color printing. Black can be
mixed from CMY when all primary slots are needed; gray can be mixed from white
and black when those two slots are the better four-color choice. Preview colors
remain estimates because physical filament opacity and transmission vary.

```sh
./btu \
    --output ~/Downloads/model-full-spectrum.3mf \
    --full-spectrum \
    ~/Downloads/model.3mf
```

When any declared source color is not one of the six available colors and
`--full-spectrum` was not supplied, an interactive terminal asks whether to
keep the mixed colors. It then opens one mapping table for all detected colors.
Each row can target any of the six colors or any other mixed color already
detected in the project, so multiple source colors can share one generated
mixture. Answering yes defaults non-base rows to their original colors;
answering no defaults them to the closest CMY replacement while still allowing
manual changes. Non-interactive runs return an error with instructions to pass
`--full-spectrum`, which keeps all detected colors without opening the table.

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
