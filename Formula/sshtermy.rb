# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.288"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.288/sshtermy_darwin_arm64"
      sha256 "9a9593d85a8ff72cf3b338f263266461e27f8aced6a67b1230f4f4bff7f7b37a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.288/sshtermy_darwin_amd64"
      sha256 "45a319c86f21c127dcab715023015699d53430f7c9060968d533825eed0385dd"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.288/sshtermy_linux_arm64"
      sha256 "7fe01f7ca1006040730860a8a3279f7365358c060dd8b8ddf7bba3a4ea367d46"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.288/sshtermy_linux_amd64"
      sha256 "734f887681cc18998833d0d232cd43c14240dfbf256069950f565d6c4a03bd7e"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
