# Wird von .github/workflows/release.yml bei jedem Release neu geschrieben.
class Sshtermy < Formula
  desc "SSH terminal with an AI copilot"
  homepage "https://db9979.github.io/sshtermy/"
  license "MIT"
  version "1.0.443"

  on_macos do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.443/sshtermy_darwin_arm64"
      sha256 "0a10c8b0e1e40d41353ca8a53c7c9705815dd7a5d8ad765643c1281727d9f177"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.443/sshtermy_darwin_amd64"
      sha256 "8cf24f3c6e7430f48810b9ae54e3d2ad00c54323fdaf3c9084696952365c538b"
    end
  end

  on_linux do
    on_arm do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.443/sshtermy_linux_arm64"
      sha256 "19601717820e692867c12b7d8387e83ff900f3ab428b2311c14af746ee10d407"
    end
    on_intel do
      url "https://github.com/db9979/sshtermy/releases/download/v1.0.443/sshtermy_linux_amd64"
      sha256 "98f5ecadd3c94a39cd57d3f5efd4db86b715b9a0e06f602adabb90f56da7961b"
    end
  end

  def install
    bin.install Dir["sshtermy_*"].first => "sshtermy"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/sshtermy --version")
  end
end
