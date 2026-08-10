# btu

[简体中文](README.zh-CN.md)

`btu` converts Bambu Studio 3MF material assignments into a Snapmaker U1 3MF
while preserving the source model geometry. It supports both ordinary
geometry-layered projects and Bambu native Full Spectrum mixed materials. For
ordinary projects, colors that cannot use a physical filament slot are
automatically converted into U1 Full Spectrum mixtures.

## Features

- Preserves source geometry, layer height, face painting, material profiles,
  and supported mixed-material definitions while replacing printer-specific
  metadata with Snapmaker U1 settings.
- Includes built-in U1 baselines for 0.2, 0.4, 0.6, and 0.8 mm nozzles, with an
  optional U1 3MF override for custom printer and process settings.
- Optimizes the four physical filament slots from actual painted-material usage
  and reports the required T1-T4 order before printing.
- Plans gray, white, and black swaps per plate for CMY-plus-neutral projects and
  prints a recommended plate order that minimizes manual filament changes.
- Generates U1 Full Spectrum virtual materials with ratio, cycle, match, and
  gradient mixing modes, including per-color mode selection in interactive use.
- Preserves all source material slots and their settings when automatic mixing
  is declined, without imposing a four-slot logical-material limit.
- Uses interactive color-plan review, progress, and overwrite confirmation in a
  terminal, while verifying the temporary 3MF before replacing an existing file.

## Conversion comparison

The example below shows the same painted model before and after conversion.
The source project uses a Bambu Lab A1 profile with five source materials. The
converted project uses the Snapmaker U1, maps directly available colors to the
four physical filament slots, and adds two Full Spectrum virtual materials for
the remaining colors. Model geometry, painting, and the source layer heights
are preserved.

| Before: Bambu Lab A1 project | After: Snapmaker U1 project converted by `btu` |
| --- | --- |
| ![Bambu Lab A1 project before conversion](docs/images/old.png) | ![Snapmaker U1 project after btu conversion](docs/images/btu.png) |

The built-in U1 baselines supply printer, process, nozzle, and U1-coupled
extrusion settings for 0.2, 0.4, 0.6, and 0.8 mm nozzles. The source supplies
geometry, layer height, material profiles, material references, face painting,
and supported mixed-material definitions. An optional U1 3MF can override the
built-in baselines.

## Install

```sh
go install github.com/nzlov/btu/cmd/btu@latest
```

This installs `btu` into `GOBIN`, or into `GOPATH/bin` when `GOBIN` is not set.
Make sure that directory is included in `PATH` before running `btu`.

## Build from source

```sh
go build -o btu ./cmd/btu
```

## Convert

The CLI display language follows `LANG`. Locale values beginning with `zh`
(such as `zh_CN.UTF-8` and `zh-TW`) use Simplified Chinese; all other values
use English.

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
| `--colors ORDER` | `-c ORDER` | no | Set the current T1-T4 color order using four distinct codes from `c`, `m`, `y`, `g`, `w`, and `b` |
| `--full-spectrum` | `-f` | no | Synthesize source colors from the loaded filaments |
| `--mix-mode MODE` | `-m MODE` | no | Set generated mixtures to `ratio`, `cycle`, `match`, or `gradient` (default: `ratio`) |
| `--[no-]subdivide-layer-height` | | no | Enable or disable all three Local-Z subdivision settings without opening their TUI |
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
slice, and nozzle-coupled extrusion metadata. Material properties such as type,
temperature, flow ratio, density, cooling, and bed temperature come from the
source and follow its material mapping into the U1 slots. When source materials
with different properties share one physical U1 component, that component keeps
the U1 baseline value instead of selecting one source profile arbitrarily. Pass
`--template` only when a different U1 3MF should override the built-in baseline.
When `--template` and `--nozzle` are used together, the template is loaded first
and the selected built-in nozzle profile is applied on top; unrelated template
settings and metadata remain.

Use four distinct characters to describe the current colors loaded in slots T1
through T4:

- `c`: cyan/blue
- `m`: magenta/red
- `y`: yellow
- `g`: gray
- `w`: white
- `b`: black

The default starting sequence is `cmyg`. For example, use `bmcy` when black,
magenta, cyan, and yellow are currently loaded in T1 through T4:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --colors bmcy \
    ~/Downloads/model.3mf
```

For ordinary projects, `btu` analyzes actual face-painted material usage and
compares every four-color subset. It prioritizes the lowest worst color error,
then weighted total error, fewer mixed components, and fewer changes from the
current slot order. The converted 3MF and `Required U1 colors` output use the
selected order. For example, a red, pink, blue, white, and black model can turn
the starting `cmyg` into `cmwb`, replacing unused yellow and gray with white and
black.

For ordinary multi-plate projects whose selected palette still contains CMY
and one neutral, `btu` reads each plate's `plater_id`, `plater_name`, object
membership, and actual material usage. It keeps the selected CMY slots fixed
and independently chooses gray, white, or black for the configured neutral
slot on each non-empty plate. The output remains one multi-plate 3MF with one
global material mapping. The CLI groups plates that use the same neutral
filament and prints a recommended order, using each plate's sequence number and
plate name. The current preview neutral group is printed first, and that slot
is replaced only between groups, minimizing filament changes without modifying
the plate order stored in the 3MF. Other four-color subsets use the one fixed
required order reported by the converter.

Orca cannot represent gray, white, and black as separate one-component virtual
materials backed by the same physical slot. A dynamic-neutral project therefore
keeps one global preview neutral, while the printed result uses the neutral
filament named in the per-plate steps. An empty plate name remains empty; `btu`
does not substitute a model name.

The converter maps the neutral selected for a plate directly to the configured
neutral slot and mixes the other mapped colors when needed. It searches recipes
against that plate's analyzed palette in the required slot order,
scores their preview colors with the same MIT-licensed FilamentMixer model used
by FullSpectrum, creates the required virtual materials, remaps model and
face-paint references, and enables U1 Local-Z mixed-color printing. If the same
source material is printed on plates that choose different neutral colors, its
shared recipe uses CMY only so that one material ID remains valid across the
project. Declared but unused colors retain deterministic mappings without
influencing a plate's neutral choice. Preview colors remain estimates because
physical filament opacity and transmission vary.

For example, a four-plate project may report:

```text
Required U1 colors: cmyg
Printing steps (recommended order, 2 T4 changes):
  keep T4 gray (cmyg): plate 4 - nose
  replace T4 gray -> white (cmyw): plate 1 - eyes, plate 3 - ears
  replace T4 white -> black (cmyb): plate 2 - body
```

```sh
./btu \
    --output ~/Downloads/model-full-spectrum.3mf \
    --full-spectrum \
    --subdivide-layer-height \
    --mix-mode match \
    ~/Downloads/model.3mf
```

Generated mixtures support the four Snapmaker Orca modes:

- `ratio` uses the closest weighted recipe with at most three filaments.
- `cycle` converts a weighted recipe of up to four filaments into a balanced,
  repeating layer pattern.
- `match` uses the closest weighted recipe with up to four filaments.
- `gradient` chooses the closest 50:50 two-filament pair and creates an A-to-B
  gradient from 80% to 20% A.

Every conversion started without `--full-spectrum` opens a color-plan review,
including projects whose colors all map directly to T1-T4. The complete source
color order is shown on the left and the planned U1 order on the right. Right-side
rows are labeled `Base`, `Material`, or `Mixed`; only mixed rows expose a mixing
mode. Use Up/Down to select an editable output, Left/Right to replace it, and
Enter to apply. On a mixed row, Tab switches focus between its mode and
replacement. A selected output can be replaced by any physical base slot or
another existing output. Every source color mapped to that output changes
together; an unneeded mixed or ordinary slot is removed and the remaining slots
are renumbered.

When a declared source color needs synthesis, the terminal first asks whether to
generate it with full-spectrum mixing. Answering yes reviews the deduplicated
mixed outputs and their preview colors. Answering no reviews the requested T1-T4
base order followed by one ordinary material slot for every source material in
source T1...Tn order. These ordinary slots retain their per-material settings and
are not limited to four logical materials. Below either color order, the same TUI
exposes three independent controls for layer-height subdivision, infill
subdivision, and whole-object subdivision. Select them with Up/Down and change
them with Left/Right or Space.
When `--full-spectrum` is supplied directly and neither layer-height flag is
present, the TUI opens a settings-only view with the same three independent
controls. `--subdivide-layer-height` enables all three together and
`--no-subdivide-layer-height` disables all three together, so non-interactive
runs can keep using one explicit flag.
Full-spectrum output also sets the wipe-tower purge volume (`prime_volume`) to
`20` mm^3. If conversion changes any supported process setting from the
selected U1 baseline, `btu` embeds a project process preset named `btu`. That
preset inherits the selected U1 system preset and stores only the changed
values, such as mixed-material definitions, Local-Z options, purge volume, or
source layer heights. If no supported process setting changes, the output keeps
the selected system preset and does not create a redundant project preset.
Non-interactive runs without `--full-spectrum` return an error because the color
plan must be reviewed. For unattended conversion, pass `--full-spectrum`, an
explicit `--subdivide-layer-height` or `--no-subdivide-layer-height`, and the
desired `--mix-mode`; that mode applies to every generated mixture.

Override the built-in baseline when needed:

```sh
./btu \
    --output ~/Downloads/model-for-u1.3mf \
    --template ~/Downloads/custom-u1.3mf \
    ~/Downloads/model.3mf
```

Progress uses Bubble Tea in an interactive terminal and plain stage messages
when output is redirected or run without a TTY. Analysis, package rewriting,
and verification report exact completed and total work-item counts, including
each model or archive member.

Replacement is published only after the temporary 3MF passes verification.
Declining the prompt leaves the existing output unchanged. Bambu
penetration-layer settings have no direct U1 equivalent and are not transferred.
Non-linear or per-part gradient curves are rejected rather than approximated. A
project that already contains native mixed materials uses its existing
definitions and must not be passed with `--full-spectrum`.
