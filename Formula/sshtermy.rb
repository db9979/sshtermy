# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.396"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.396/sshtermy_darwin_arm64"
      sha256 "32f90ab979b54a4999d30b3ffb33c4ca238949c1a38b0bb9ee7ea20e121d783b"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.396/sshtermy_darwin_amd64"
      sha256 "40634f1676c0e548efe01a01e910648dc968c65e9544ed3cb72dd2f3d918c252"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.396/sshtermy_linux_arm64"
      sha256 "974669f743b24b5c43781601231f41288d19248b90628ee2f5684c6059054e7a"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.396/sshtermy_linux_amd64"
      sha256 "f5cfa065c584e582161e55e15eaf2bceb261d197f32aad8333f30f259a4f1b6a"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
