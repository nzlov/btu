# btu

`btu` converts Bambu Studio 3MF material assignments into a Snapmaker U1 3MF
while preserving the source model geometry. It supports both ordinary
geometry-layered projects and Bambu native Full Spectrum mixed materials.

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

Without `--output`, this writes `~/Downloads/Fantasmino-btu.3mf`. Existing
outputs are never overwritten.

Options:

| Long | Short | Required | Purpose |
| --- | --- | --- | --- |
| `--output FILE` | `-o FILE` | no | Override the default `SOURCE-btu.3mf` path |
| `--colors WRBYK` | `-c WRBYK` | no | Colors loaded in slots 1 through 4 |
| `--full-spectrum` | `-f` | no | Synthesize source colors from the loaded filaments |
| `--nozzle SIZE` | `-n SIZE` | no | Force the 0.2, 0.4, 0.6, or 0.8 mm built-in baseline |
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

Use four characters to describe the actual colors loaded in slots 1 through 4:

- `w`: white
- `r`: red
- `b`: blue
- `y`: yellow
- `k`: black

The default CMYK-style sequence is blue, red, yellow, and black: `bryk`. Each
color can appear at most once. For example, black, red, yellow, and blue in
slots 1 through 4 is `kryb`:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --colors kryb \
    ~/Downloads/model.3mf
```

For an ordinary project whose source colors are not limited to the four loaded
filaments, enable full-spectrum synthesis. The converter decomposes each source
color into at most three neutral/red/yellow/blue components using an RYB model,
creates the required virtual materials, remaps model and face-paint references,
and enables U1 Local-Z mixed-color printing. For example, pure green becomes a
50/50 yellow and blue mix.

```sh
./btu \
    --output ~/Downloads/model-full-spectrum.3mf \
    --full-spectrum \
    ~/Downloads/model.3mf
```

Full-spectrum synthesis requires red, yellow, blue, and exactly one neutral
filament: white or black. The default `bryk` uses black automatically as the
neutral filament; there is no separate white-to-black option.

Override the built-in baseline when needed:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --template ~/Downloads/custom-u1.3mf \
    ~/Downloads/model.3mf
```

Progress uses Bubble Tea in an interactive terminal and plain stage messages
when output is redirected or run without a TTY.

The converter does not overwrite an existing output. Bambu penetration-layer
settings have no direct U1 equivalent and are not transferred. Non-linear or
per-part gradient curves are rejected rather than approximated. A project that
already contains native mixed materials uses its existing definitions and must
not be passed with `--full-spectrum`.
