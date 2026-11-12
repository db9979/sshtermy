# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.364"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.364/sshtermy_darwin_arm64"
      sha256 "a69e7f4570ee04602739c4864f087dc3a93955c4dbd305b7de0638015d585c1d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.364/sshtermy_darwin_amd64"
      sha256 "bf451157280b7fa02b9c70d599715568f1f61d8886ab9fd32700066ccc62918c"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.364/sshtermy_linux_arm64"
      sha256 "5915e3811f86c711e3851fc7419f0644ed84a8d8d0836e3f33409c7643715f2d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.364/sshtermy_linux_amd64"
      sha256 "a82ae8bc39572cecbf1b88dbc8617c0046b4d853c057f6c8a729ddff3099b798"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
