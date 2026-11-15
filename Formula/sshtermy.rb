# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.413"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.413/sshtermy_darwin_arm64"
      sha256 "8ace77efceb6304dcc5e86d2be6e763a1fdd9905018ead78daba60eefef8ceed"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.413/sshtermy_darwin_amd64"
      sha256 "132f425292f1341b4dfd68e0f86a1129115bbc613a3fca8671a7ba7b697a718e"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.413/sshtermy_linux_arm64"
      sha256 "bbc26024aeca5a97e30ee580960a80afbd4a2d1bdbf594f65fa60b3826c05d88"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.413/sshtermy_linux_amd64"
      sha256 "01488a11d63b5c2c106a28d7da57cc14c87fa6ad200b820297a9ab4d7cfae72c"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
