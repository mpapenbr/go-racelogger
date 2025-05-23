# go-racelogger

<div align="center">

[![Build Status](https://img.shields.io/github/checks-status/mpapenbr/go-racelogger/main?color=black&style=for-the-badge&logo=github)][github-actions]
[![Security: bandit](https://img.shields.io/badge/Security-GoSec-lightgrey?style=for-the-badge&logo=springsecurity)](https://github.com/securego/gosec)
[![Dependencies Status](https://img.shields.io/badge/Dependencies-Up%20to%20Date-brightgreen?style=for-the-badge&logo=dependabot)][dependabot-pulls]
[![Semantic Versioning](https://img.shields.io/badge/versioning-semantic-black?style=for-the-badge&logo=semver)][github-releases]
[![License](https://img.shields.io/github/license/mpapenbr/go-racelogger?color=red&style=for-the-badge)][project-license]
[![Go v1.24](https://img.shields.io/badge/Go-%20v1.24-black?style=for-the-badge&logo=go)][gomod-file]

Racelogger for iRacelog project

</div>

## Initial Setup

This section is intended to help developers and contributors get a working copy of
`go-racelogger` on their end

<details>
<summary>
    Clone this repository
</summary><br>

```sh
git clone https://github.com/mpapenbr/go-racelogger
cd go-racelogger
```

</details>

<details>
<summary>
    Option: Do you own setup
</summary><br>

Install `golangci-lint` from the [official website][golangci-install] for your OS

</details>
<br>

## Local Development

This section will guide you to setup a fully-functional local copy of `go-racelogger`
on your end and teach you how to use it! Make sure you have installed
[golangci-lint][golangci-install] before following this section!

**Note:** This section relies on the usage of [Makefile][makefile-official]. If you
can't (or don't) use Makefile, you can follow along by running the internal commands
from [`go-racelogger's` Makefile][makefile-file] (most of which are
OS-independent)!

### Installing dependencies

To install all dependencies associated with `go-racelogger`, run the
command

```sh
make install
```

### Using Code Formatters

Code formatters format your code to match pre-decided conventions. To run automated code
formatters, use the Makefile command

```sh
make codestyle
```

### Using Code Linters

Linters are tools that analyze source code for possible errors. This includes typos,
code formatting, syntax errors, calls to deprecated functions, potential security
vulnerabilities, and more!

To run pre-configured linters, use the command

```sh
make lint
```

### Running Tests

Tests in `go-racelogger` are classified as _fast_ and _slow_ - depending
on how quick they are to execute.

To selectively run tests from either test group, use the Makefile command

```sh
make fast-test

OR

make slow-test
```

Alternatively, to run the complete test-suite -- i.e. _fast_ and _slow_ tests at one
go, use the command

```sh
make test
```

### Running the Test-Suite

The _`test-suite`_ is simply a wrapper to run linters, stylecheckers and **all** tests
at once!

To run the test-suite, use the command

```sh
make test-suite
```

In simpler terms, running the test-suite is a combination of running [linters](#using-code-linters)
and [all tests](#running-tests) one after the other!
<br>

## Additional Resources

### Makefile help

<details>
<summary>
    Tap for a list of Makefile commands
</summary><br>

| Command |                             Description                             | Prerequisites |
| :-----: | :-----------------------------------------------------------------: | :-----------: |
| `help`  | Generate help dialog listing all Makefile commands with description |      NA       |

| `install` | Fetch project dependencies | NA |
| `codestyle` | Run code-formatters | golangci-lint |
| `lint` | Check codestyle and run linters | golangci-lint |
| `test` | Run **all** tests | NA |
| `fast-tests` | Selectively run _fast_ tests | NA |
| `slow-tests` | Selectively run _slow_ tests | NA |
| `test-suite` | Check codestyle, run linters and **all** tests | golangci-lint |
| `run` | Run _go-racelogger_ | NA |

<br>
</details>

Optionally, to see a list of all Makefile commands, and a short description of what they
do, you can simply run

```sh
make
```

Which is equivalent to;

```sh
make help
```

Both of which will list out all Makefile commands available, and a short description
of what they do!

### Generating Binaries

To generate binaries for multiple OS/architectures, simply run

```sh
goreleaser build
```

The command will generate binaries for Linux, Windows and Mac targetting multiple
architectures at once! The binaries, once generated will be stored in the `dist`
directory inside the project directory.

Adjust the .goreleaser.yml to fit your needs.

See [goreleaser] for details.

### Generating Images

[goreleaser] is also used to create archives and docker images. This can be done by

```sh
goreleaser release
```

The current `.goreleaser.yml` is target for creating docker images and artefacts to be created by Github actions.

### Running `go-racelogger`

To run go-racelogger, use the command

```sh
make run
```

Additionally, you can pass any additional command-line arguments (if needed) as the
argument "`q`". For example;

```sh
make run q="--help"

OR

make run q="--version"
```

<br>

## Releases

You can check out a list of previous releases on the [Github Releases][github-releases]
page.

### Semantic versioning with Release Drafter

<details>
    <summary>
        What is Semantic Versioning?
    </summary><br>

Semantic versioning is a versioning scheme aimed at making software management easier.
Following semantic versioning, version identifiers are divided into three parts;

```sh
    <major>.<minor>.<patch>
```

> MAJOR version when you make incompatible API changes [breaking changes]<br>
> MINOR version when you add functionality in a backwards compatible manner [more features]<br>
> PATCH version when you make backwards compatible bug fixes [bug fixes and stuff]<br>

For a more detailed description, head over to [semver.org][semver-link]

</details>

[Release Drafter][release-drafter] automatically updates the release version as pull
requests are merged.

Labels allowed;

-   `major`: Affects the `<major>` version number for semantic versioning
-   `minor`, `enhancement`, `update`, `feature`: Affects the `<minor>` version number for semantic versioning
-   all other labels affect the `<patch>` version number

Whenever a pull request with one of these labels is merged to the `master` branch,
the corresponding version number will be bumped by one digit!

### List of Labels

Pull requests once merged, will be classified into categories by
[release-drafter][release-drafter] based on pull request labels

This is managed by the [`release-drafter.yml`][release-drafter-config] config file.

|                        **Label**                        |      **Title in Releases**      |
| :-----------------------------------------------------: | :-----------------------------: |
|                       `security`                        |         :lock: Security         |
|           `enhancement`, `feature`, `update`            |        :rocket: Updates         |
|                 `bug`, `bugfix`, `fix`                  |         :bug: Bug Fixes         |
|                 `documentation`, `docs`                 |      :memo: Documentation       |
| `wip`, `in-progress`, `incomplete`, `partial`, `hotfix` | :construction: Work in Progress |
|              `dependencies`, `dependency`               |     :package: Dependencies      |
|      `refactoring`, `refactor`, `tests`, `testing`      | :test_tube: Tests and Refactor  |
|                `build`, `ci`, `pipeline`                |   :robot: CI/CD and Pipelines   |

The labels `bug`, `enhancement`, and `documentation` are automatically created by Github
for repositories. [Dependabot][dependabot-link] will implicitly create the
`dependencies` label with the first pull request raised by it.

The remaining labels can be created as needed!
<br>

## Credits

<div align="center"><br>

`go-racelogger` is powered by a template generated using [`cookiecutter-go`][cookiecutter-go-link] which is based on [`go-template`][go-template-link]

[![cookiecutter-go-link](https://img.shields.io/badge/cookiecutter--go-black?style=for-the-badge&logo=go)][cookiecutter-go-link]
[![go-template](https://img.shields.io/badge/go--template-black?style=for-the-badge&logo=go)][go-template-link]

</div>

[makefile-file]: ./Makefile
[project-license]: ./LICENSE
[github-actions]: ../../actions
[github-releases]: ../../releases
[precommit-config]: ./.pre-commit-config.yaml
[gomod-file]: ../main/go.mod
[github-actions-tests]: ../../actions/workflows/tests.yml
[dependabot-pulls]: ../../pulls?utf8=%E2%9C%93&q=is%3Apr%20author%3Aapp%2Fdependabot
[semver-link]: https://semver.org
[pre-commit]: https://pre-commit.com
[github-repo]: https://github.com/new
[gitlab-repo]: https://gitlab.com/new
[dependabot-link]: https://dependabot.com
[githooks]: https://git-scm.com/docs/githooks
[goreleaser]: https://goreleaser.com/intro/
[python-install]: https://www.python.org/downloads
[release-drafter-config]: ./.github/release-drafter.yml
[makefile-official]: https://www.gnu.org/software/make
[pip-install]: https://pip.pypa.io/en/stable/installation
[codecov-docs]: https://docs.codecov.com/docs#basic-usage
[go-template-link]: https://github.com/notsatan/go-template
[golangci-install]: https://golangci-lint.run/usage/install
[cookiecutter-link]: https://github.com/cookiecutter/cookiecutter
[cookiecutter-go-link]: https://github.com/mpapenbr/cookiecutter-go
[release-drafter]: https://github.com/marketplace/actions/release-drafter
[creating-secrets]: https://docs.github.com/en/actions/security-guides/encrypted-secrets#creating-encrypted-secrets-for-a-repository
[linting-golang]: https://freshman.tech/linting-golang/
