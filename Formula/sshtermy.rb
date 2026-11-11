# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.342"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.342/sshtermy_darwin_arm64"
      sha256 "684784a2f333faa0718de708bd33cf615c03febf0081766e4f9b504257985f3e"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.342/sshtermy_darwin_amd64"
      sha256 "5f0e2db823053fdc7e6a1543772a7adbb44654492401b262b30d0ca6f991d4dc"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.342/sshtermy_linux_arm64"
      sha256 "65eead4101329076792d60e8dbc30317366a5db1be910023a78f5ecac65fe097"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.342/sshtermy_linux_amd64"
      sha256 "4f48af19dc981bd8fb4127aa4adca5e8166edb8b27bd3b1cce07dc4ddab4a35d"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
