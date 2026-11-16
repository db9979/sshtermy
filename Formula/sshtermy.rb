# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.416"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.416/sshtermy_darwin_arm64"
      sha256 "80b8f40b115118ecfb7a59d854788e729268b631d7caff19e23b83f924f47cb6"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.416/sshtermy_darwin_amd64"
      sha256 "42f4c99da7085a2dbac868de355aca6ec793e95aa716ac6c028ce0d0d892a68a"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.416/sshtermy_linux_arm64"
      sha256 "9f8ccb7636c0e5135a2cbbdd70bab9e34a3939cc6fc09403d07f72a5db656138"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.416/sshtermy_linux_amd64"
      sha256 "b078c4ebecbd782e2fa866c4e84f73a31e5e496018a92e798c4ec95df5176217"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
