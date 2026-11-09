# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.302"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.302/sshtermy_darwin_arm64"
      sha256 "163bf75eac7f174412f74f8dc12049a4b6fb178dd2cd0b0f811ef2224426093d"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.302/sshtermy_darwin_amd64"
      sha256 "7ef0c55482620db9e93b6b8ca4a44d038a10787b09383132e2ff18da5f121ce8"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.302/sshtermy_linux_arm64"
      sha256 "122987057b002e3dc3c5199134d12580ceacc1926172d17732cd8ac252124e70"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.302/sshtermy_linux_amd64"
      sha256 "b12c26e1fa82a9f3a976473673be7af163985d0fd3d653db711c61c6626f976c"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
