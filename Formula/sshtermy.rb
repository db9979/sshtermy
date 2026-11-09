# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.293"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.293/sshtermy_darwin_arm64"
      sha256 "c698ee3de9e48a943e4461f8834480370a0230a6beae560ef6d9360ce531837d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.293/sshtermy_darwin_amd64"
      sha256 "d9446344703cf8cb1ea1683eaf410cfb1cc04f6ef6b0d69615335fab4c306766"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.293/sshtermy_linux_arm64"
      sha256 "b45b5bed3b753930e6d97c4f9b308ebc5dec5e14103f603af5f583abce27477b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.293/sshtermy_linux_amd64"
      sha256 "b01dccd573b661aaaaaed5e7b456a034437e223b3612fc37ebd77c659811ad88"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
