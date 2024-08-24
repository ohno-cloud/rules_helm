load("@aspect_bazel_lib//lib:write_source_files.bzl", "write_source_files")

def _generate_lockfile(ctx):
    output_file = ctx.actions.declare_file(ctx.label.name + ".lock.json")

    args = ctx.actions.args()

    args.add_all("-chart", ctx.files.charts_file)
    args.add("-output", output_file)

    ctx.actions.run(
        mnemonic = "HelmLockFile",
        executable = ctx.executable._helmlock,
        arguments = [args],
        inputs = ctx.files.charts_file,
        outputs = [output_file],
    )

    return [
         DefaultInfo(files = depset([output_file])),
    ]

generate_lockfile = rule(
    implementation = _generate_lockfile,
    attrs = {
        "charts_file": attr.label(allow_files = [".yaml"]),
        "_helmlock": attr.label(
            executable=True,
            cfg = "exec",
            default="//cmd/helmlock"
        ),
    },
)


def helm_lock_file(name, charts_file, lock_file):

    generate_lockfile(
        name=name,
        charts_file=charts_file,
        tags = [
            "requires-network",
            "no-remote-exec",
            "no-sandbox",
        ],
    )

    write_source_files(
        name = name + ".update",
        files = {
            lock_file: ":" + name,
        },
    )
