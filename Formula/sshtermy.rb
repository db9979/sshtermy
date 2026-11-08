# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.267"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.267/sshtermy_darwin_arm64"
      sha256 "dbffdb80737f769d6390cdb68d35fed6978b6cb0896535fc53136579f8d708b6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.267/sshtermy_darwin_amd64"
      sha256 "ea35e9d92393784f2e1680cb021e55ca07a97e4b13c51b396d46206026bc8eeb"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.267/sshtermy_linux_arm64"
      sha256 "a03f3164f00953ac2a89236366bf29d4ba2906aba05315420fedc3f3e570bea3"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.267/sshtermy_linux_amd64"
      sha256 "7049af8cbdb54171bc847345de3e12529c1cdbfa8df880228d67dbb962ef62d0"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
